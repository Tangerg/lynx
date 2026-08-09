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
	counts := make(map[string]int, len(plugins))
	index := make(map[string]int, len(plugins))
	byID := make(map[string]Plugin, len(plugins))
	for i, plugin := range plugins {
		counts[plugin.ID]++
		if _, exists := index[plugin.ID]; !exists {
			index[plugin.ID] = i
			byID[plugin.ID] = clonePlugin(plugin)
		}
	}

	result := resolution{}
	skipped := make(map[string]error)
	for _, plugin := range plugins {
		if _, handled := skipped[plugin.ID]; handled {
			continue
		}
		if counts[plugin.ID] > 1 {
			result.plugins = append(result.plugins, clonePlugin(plugin))
			skipped[plugin.ID] = fmt.Errorf("extensions: plugin id %q is declared %d times", plugin.ID, counts[plugin.ID])
			continue
		}
		result.plugins = append(result.plugins, clonePlugin(plugin))
		if err := ValidateManifest(plugin); err != nil {
			skipped[plugin.ID] = err
		}
	}

	for _, plugin := range result.plugins {
		if skipped[plugin.ID] != nil {
			continue
		}
		for _, dependency := range plugin.Requires {
			if counts[dependency] != 1 {
				skipped[plugin.ID] = fmt.Errorf("extensions: plugin %q requires unavailable plugin %q", plugin.ID, dependency)
				break
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, plugin := range result.plugins {
			if skipped[plugin.ID] != nil {
				continue
			}
			for _, dependency := range plugin.Requires {
				if skipped[dependency] != nil {
					skipped[plugin.ID] = fmt.Errorf("extensions: plugin %q requires skipped plugin %q", plugin.ID, dependency)
					changed = true
					break
				}
			}
		}
	}

	degree := make(map[string]int, len(result.plugins))
	dependents := make(map[string][]string, len(result.plugins))
	for _, plugin := range result.plugins {
		if skipped[plugin.ID] == nil {
			degree[plugin.ID] = 0
		}
	}
	for _, plugin := range result.plugins {
		if skipped[plugin.ID] != nil {
			continue
		}
		for _, dependency := range plugin.Requires {
			degree[plugin.ID]++
			dependents[dependency] = append(dependents[dependency], plugin.ID)
		}
	}

	ready := make([]string, 0, len(degree))
	for id, count := range degree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	for len(ready) > 0 {
		slices.SortStableFunc(ready, func(a, b string) int { return index[a] - index[b] })
		id := ready[0]
		ready = ready[1:]
		result.order = append(result.order, clonePlugin(byID[id]))
		for _, dependent := range dependents[id] {
			degree[dependent]--
			if degree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	for id, count := range degree {
		if count > 0 {
			skipped[id] = errors.New("extensions: dependency cycle")
		}
	}
	for _, plugin := range plugins {
		if err := skipped[plugin.ID]; err != nil {
			result.issues = append(result.issues, ResolutionIssue{PluginID: plugin.ID, Err: err})
			delete(skipped, plugin.ID)
		}
	}
	return result
}
