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

type resolution struct {
	plugins []Plugin
	order   []Plugin
	issues  []ResolutionIssue
}

func resolve(plugins []Plugin) resolution {
	state := newResolver(plugins)
	state.collectCandidates(plugins)
	state.rejectUnavailableDependencies()
	state.propagateSkippedDependencies()
	degree, dependents := state.dependencyGraph()
	state.resolveOrder(degree, dependents)
	state.rejectCycles(degree)
	state.collectIssues(plugins)
	return state.result
}

type resolver struct {
	counts  map[string]int
	index   map[string]int
	byID    map[string]Plugin
	skipped map[string]error
	result  resolution
}

func newResolver(plugins []Plugin) *resolver {
	state := &resolver{
		counts: make(map[string]int, len(plugins)), index: make(map[string]int, len(plugins)),
		byID: make(map[string]Plugin, len(plugins)), skipped: make(map[string]error),
	}
	for i, plugin := range plugins {
		state.counts[plugin.ID]++
		if _, exists := state.index[plugin.ID]; !exists {
			state.index[plugin.ID] = i
			state.byID[plugin.ID] = clonePlugin(plugin)
		}
	}
	return state
}

func (r *resolver) collectCandidates(plugins []Plugin) {
	included := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		if _, exists := included[plugin.ID]; exists {
			continue
		}
		included[plugin.ID] = struct{}{}
		r.result.plugins = append(r.result.plugins, clonePlugin(plugin))
		if r.counts[plugin.ID] > 1 {
			r.skipped[plugin.ID] = fmt.Errorf("extensions: plugin id %q is declared %d times", plugin.ID, r.counts[plugin.ID])
			continue
		}
		if err := ValidateManifest(plugin); err != nil {
			r.skipped[plugin.ID] = err
		}
	}
}

func (r *resolver) rejectUnavailableDependencies() {
	for _, plugin := range r.result.plugins {
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

func (r *resolver) propagateSkippedDependencies() {
	for changed := true; changed; {
		changed = false
		for _, plugin := range r.result.plugins {
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

func (r *resolver) dependencyGraph() (map[string]int, map[string][]string) {
	degree := make(map[string]int, len(r.result.plugins))
	dependents := make(map[string][]string, len(r.result.plugins))
	for _, plugin := range r.result.plugins {
		if r.skipped[plugin.ID] == nil {
			degree[plugin.ID] = 0
		}
	}
	for _, plugin := range r.result.plugins {
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

func (r *resolver) resolveOrder(degree map[string]int, dependents map[string][]string) {
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
		r.result.order = append(r.result.order, clonePlugin(r.byID[id]))
		for _, dependent := range dependents[id] {
			degree[dependent]--
			if degree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
}

func (r *resolver) rejectCycles(degree map[string]int) {
	for id, count := range degree {
		if count > 0 {
			r.skipped[id] = errors.New("extensions: dependency cycle")
		}
	}
}

func (r *resolver) collectIssues(plugins []Plugin) {
	for _, plugin := range plugins {
		if err := r.skipped[plugin.ID]; err != nil {
			r.result.issues = append(r.result.issues, ResolutionIssue{PluginID: plugin.ID, Err: err})
			delete(r.skipped, plugin.ID)
		}
	}
}
