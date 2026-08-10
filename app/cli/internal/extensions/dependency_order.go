package extensions

import (
	"errors"
	"fmt"
	"slices"
)

// ResolutionIssue explains why one manifest cannot participate in activation.
type ResolutionIssue struct {
	PluginID string
	Err      error
}

type dependencyResolution struct {
	plugins []Plugin
	order   []Plugin
	issues  []ResolutionIssue
}

func resolve(plugins []Plugin) dependencyResolution {
	resolver := newDependencyResolver(plugins)
	resolver.collectCandidates(plugins)
	resolver.rejectUnavailableDependencies()
	resolver.propagateSkippedDependencies()
	degree, dependents := resolver.dependencyGraph()
	resolver.resolveOrder(degree, dependents)
	resolver.rejectCycles(degree)
	resolver.collectIssues(plugins)
	return resolver.resolution
}

type dependencyResolver struct {
	counts     map[string]int
	index      map[string]int
	byID       map[string]Plugin
	skipped    map[string]error
	resolution dependencyResolution
}

func newDependencyResolver(plugins []Plugin) *dependencyResolver {
	resolver := &dependencyResolver{
		counts: make(map[string]int, len(plugins)), index: make(map[string]int, len(plugins)),
		byID: make(map[string]Plugin, len(plugins)), skipped: make(map[string]error),
	}
	for i, plugin := range plugins {
		resolver.counts[plugin.ID]++
		if _, exists := resolver.index[plugin.ID]; !exists {
			resolver.index[plugin.ID] = i
			resolver.byID[plugin.ID] = clonePlugin(plugin)
		}
	}
	return resolver
}

func (r *dependencyResolver) collectCandidates(plugins []Plugin) {
	included := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		if _, exists := included[plugin.ID]; exists {
			continue
		}
		included[plugin.ID] = struct{}{}
		r.resolution.plugins = append(r.resolution.plugins, clonePlugin(plugin))
		if r.counts[plugin.ID] > 1 {
			r.skipped[plugin.ID] = fmt.Errorf("extensions: plugin id %q is declared %d times", plugin.ID, r.counts[plugin.ID])
			continue
		}
		if err := ValidateManifest(plugin); err != nil {
			r.skipped[plugin.ID] = err
		}
	}
}

func (r *dependencyResolver) rejectUnavailableDependencies() {
	for _, plugin := range r.resolution.plugins {
		if r.skipped[plugin.ID] != nil {
			continue
		}
		for _, dependency := range plugin.Requires {
			if r.counts[dependency] != 1 {
				r.skipped[plugin.ID] = fmt.Errorf("extensions: plugin %q requires unavailable plugin %q", plugin.ID, dependency)
				break
			}
		}
	}
}

func (r *dependencyResolver) propagateSkippedDependencies() {
	for changed := true; changed; {
		changed = false
		for _, plugin := range r.resolution.plugins {
			if r.skipped[plugin.ID] != nil {
				continue
			}
			for _, dependency := range plugin.Requires {
				if r.skipped[dependency] != nil {
					r.skipped[plugin.ID] = fmt.Errorf("extensions: plugin %q requires skipped plugin %q", plugin.ID, dependency)
					changed = true
					break
				}
			}
		}
	}
}

func (r *dependencyResolver) dependencyGraph() (map[string]int, map[string][]string) {
	degree := make(map[string]int, len(r.resolution.plugins))
	dependents := make(map[string][]string, len(r.resolution.plugins))
	for _, plugin := range r.resolution.plugins {
		if r.skipped[plugin.ID] == nil {
			degree[plugin.ID] = 0
		}
	}
	for _, plugin := range r.resolution.plugins {
		if r.skipped[plugin.ID] != nil {
			continue
		}
		for _, dependency := range plugin.Requires {
			degree[plugin.ID]++
			dependents[dependency] = append(dependents[dependency], plugin.ID)
		}
	}
	return degree, dependents
}

func (r *dependencyResolver) resolveOrder(degree map[string]int, dependents map[string][]string) {
	ready := make([]string, 0, len(degree))
	for id, count := range degree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	for len(ready) > 0 {
		slices.SortStableFunc(ready, func(a, b string) int { return r.index[a] - r.index[b] })
		id := ready[0]
		ready = ready[1:]
		r.resolution.order = append(r.resolution.order, clonePlugin(r.byID[id]))
		for _, dependent := range dependents[id] {
			degree[dependent]--
			if degree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
}

func (r *dependencyResolver) rejectCycles(degree map[string]int) {
	for id, count := range degree {
		if count > 0 {
			r.skipped[id] = errors.New("extensions: dependency cycle")
		}
	}
}

func (r *dependencyResolver) collectIssues(plugins []Plugin) {
	for _, plugin := range plugins {
		if err := r.skipped[plugin.ID]; err != nil {
			r.resolution.issues = append(r.resolution.issues, ResolutionIssue{PluginID: plugin.ID, Err: err})
			delete(r.skipped, plugin.ID)
		}
	}
}
