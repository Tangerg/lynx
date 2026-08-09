package extensions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

var ErrPluginNotFound = errors.New("extensions: plugin not found")

type Phase string

const (
	PluginAvailable Phase = "available"
	PluginLoaded    Phase = "loaded"
	PluginSkipped   Phase = "skipped"
	PluginFailed    Phase = "failed"
)

// Result is the outcome of one activation or lifecycle operation.
type Result struct {
	PluginID string
	Phase    Phase
	Err      error
}

// Info is a read-only plugin lifecycle snapshot.
type Info struct {
	ID           string
	Version      string
	APIVersion   int
	Requires     []string
	Capabilities []Capability
	Trusted      bool
	Phase        Phase
	Detail       string
}

// Kernel owns discovered manifests and loaded plugin lifetimes. Its zero value
// is not usable because a registry is an explicit composition dependency.
type Kernel struct {
	lifecycle sync.Mutex
	mu        sync.RWMutex
	registry  *Registry
	plugins   map[string]Plugin
	loaded    map[string]*Loaded
	states    map[string]Result
	order     []string
	activated bool
	closed    bool
}

func NewKernel(registry *Registry) (*Kernel, error) {
	if registry == nil {
		return nil, errors.New("extensions: registry is required")
	}
	return &Kernel{
		registry: registry,
		plugins:  make(map[string]Plugin),
		loaded:   make(map[string]*Loaded),
		states:   make(map[string]Result),
	}, nil
}

// Activate validates, orders, and loads one discovered plugin set. Individual
// failures are isolated and returned as results; calling Activate twice would
// make ownership ambiguous and is rejected.
func (k *Kernel) Activate(plugins []Plugin) ([]Result, error) {
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil, errKernelClosed
	}
	if k.activated {
		k.mu.Unlock()
		return nil, errors.New("extensions: kernel is already activated")
	}
	k.activated = true

	resolved := resolve(plugins)
	for _, plugin := range resolved.plugins {
		k.plugins[plugin.ID] = clonePlugin(plugin)
		k.states[plugin.ID] = Result{PluginID: plugin.ID, Phase: PluginAvailable}
	}
	for _, issue := range resolved.issues {
		k.states[issue.PluginID] = Result{PluginID: issue.PluginID, Phase: PluginSkipped, Err: issue.Err}
	}
	for _, plugin := range resolved.order {
		k.order = append(k.order, plugin.ID)
	}
	order := slices.Clone(k.order)
	k.mu.Unlock()

	results := make([]Result, 0, len(plugins))
	for _, issue := range resolved.issues {
		results = append(results, Result{PluginID: issue.PluginID, Phase: PluginSkipped, Err: issue.Err})
	}
	for _, id := range order {
		results = append(results, k.load(id))
	}
	return results, nil
}

// Infos returns manifests in resolved load order followed by structurally
// skipped plugins in stable identity order.
func (k *Kernel) Infos() []Info {
	k.mu.RLock()
	defer k.mu.RUnlock()
	infos := make([]Info, 0, len(k.plugins))
	seen := make(map[string]struct{}, len(k.plugins))
	for _, id := range k.order {
		infos = append(infos, k.infoLocked(id))
		seen[id] = struct{}{}
	}
	for id := range k.plugins {
		if _, exists := seen[id]; !exists {
			infos = append(infos, k.infoLocked(id))
		}
	}
	slices.SortStableFunc(infos[len(k.order):], func(a, b Info) int { return strings.Compare(a.ID, b.ID) })
	return infos
}

func (k *Kernel) infoLocked(id string) Info {
	plugin := k.plugins[id]
	state := k.states[id]
	detail := ""
	if state.Err != nil {
		detail = state.Err.Error()
	}
	return Info{
		ID: plugin.ID, Version: plugin.Version, APIVersion: plugin.APIVersion,
		Requires: slices.Clone(plugin.Requires), Capabilities: slices.Clone(plugin.Capabilities),
		Trusted: plugin.Trusted, Phase: state.Phase, Detail: detail,
	}
}

// Reload unloads a plugin and every transitive dependent, then reactivates the
// closure in dependency order so no dependent retains stale registrations.
func (k *Kernel) Reload(id string) ([]Result, error) {
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil, errKernelClosed
	}
	if _, exists := k.plugins[id]; !exists {
		k.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	closure := k.dependentClosureLocked(id)
	order := slices.Clone(k.order)
	k.mu.Unlock()
	k.unloadSet(closure)
	results := make([]Result, 0, len(closure))
	for _, candidate := range order {
		if closure[candidate] {
			results = append(results, k.load(candidate))
		}
	}
	if len(results) == 0 {
		k.mu.RLock()
		state := k.states[id]
		k.mu.RUnlock()
		if state.Err == nil {
			state.Err = errors.New("extensions: plugin is not in the resolved dependency graph")
		}
		return []Result{state}, nil
	}
	return results, nil
}

// Unload removes a plugin and its transitive dependents in reverse order.
func (k *Kernel) Unload(id string) error {
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return errKernelClosed
	}
	if _, exists := k.plugins[id]; !exists {
		k.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	closure := k.dependentClosureLocked(id)
	k.mu.Unlock()
	k.unloadSet(closure)
	return nil
}

func (k *Kernel) dependentClosureLocked(id string) map[string]bool {
	closure := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for candidate, plugin := range k.plugins {
			if closure[candidate] {
				continue
			}
			if slices.ContainsFunc(plugin.Requires, func(dependency string) bool { return closure[dependency] }) {
				closure[candidate] = true
				changed = true
			}
		}
	}
	return closure
}

func (k *Kernel) unloadSet(ids map[string]bool) {
	k.mu.Lock()
	var disposables []*Loaded
	for i := len(k.order) - 1; i >= 0; i-- {
		id := k.order[i]
		if !ids[id] {
			continue
		}
		if loaded := k.loaded[id]; loaded != nil {
			disposables = append(disposables, loaded)
			delete(k.loaded, id)
		}
		k.states[id] = Result{PluginID: id, Phase: PluginAvailable}
	}
	k.mu.Unlock()
	for _, loaded := range disposables {
		loaded.Dispose()
	}
}

func (k *Kernel) load(id string) Result {
	k.mu.Lock()
	plugin, exists := k.plugins[id]
	if !exists {
		k.mu.Unlock()
		return Result{PluginID: id, Phase: PluginFailed, Err: fmt.Errorf("%w: %s", ErrPluginNotFound, id)}
	}
	if loaded := k.loaded[id]; loaded != nil {
		result := Result{PluginID: id, Phase: PluginLoaded}
		k.states[id] = result
		k.mu.Unlock()
		return result
	}
	for _, dependency := range plugin.Requires {
		if k.loaded[dependency] == nil {
			result := Result{
				PluginID: id, Phase: PluginSkipped,
				Err: fmt.Errorf("extensions: plugin %q requires inactive plugin %q", id, dependency),
			}
			k.states[id] = result
			k.mu.Unlock()
			return result
		}
	}
	k.mu.Unlock()
	loaded, err := Load(k.registry, plugin)
	k.mu.Lock()
	defer k.mu.Unlock()
	if err != nil {
		result := Result{PluginID: id, Phase: PluginFailed, Err: err}
		k.states[id] = result
		return result
	}
	k.loaded[id] = loaded
	result := Result{PluginID: id, Phase: PluginLoaded}
	k.states[id] = result
	return result
}

// Close unloads every plugin in reverse dependency order. It is idempotent.
func (k *Kernel) Close() {
	if k == nil {
		return
	}
	k.lifecycle.Lock()
	defer k.lifecycle.Unlock()
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return
	}
	all := make(map[string]bool, len(k.plugins))
	for id := range k.plugins {
		all[id] = true
	}
	k.closed = true
	k.mu.Unlock()
	k.unloadSet(all)
}
