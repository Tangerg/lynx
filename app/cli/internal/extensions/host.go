package extensions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

var (
	ErrPluginNotFound = errors.New("extensions: plugin not found")
	errHostClosed     = errors.New("extensions: host is closed")
)

type LifecyclePhase string

const (
	PluginAvailable LifecyclePhase = "available"
	PluginLoaded    LifecyclePhase = "loaded"
	PluginSkipped   LifecyclePhase = "skipped"
	PluginFailed    LifecyclePhase = "failed"
)

// LifecycleResult is the outcome of one activation or lifecycle operation.
type LifecycleResult struct {
	PluginID string
	Phase    LifecyclePhase
	Err      error
}

// Status is a read-only plugin lifecycle snapshot.
type Status struct {
	ID           string
	Version      string
	APIVersion   int
	Requires     []string
	Capabilities []Capability
	Trusted      bool
	Phase        LifecyclePhase
	Detail       string
}

// Host owns discovered manifests and loaded plugin lifetimes. Its zero value
// is not usable because a registry is an explicit composition dependency.
type Host struct {
	lifecycleMu sync.Mutex
	stateMu     sync.RWMutex
	registry    *Registry
	plugins     map[string]Plugin
	loaded      map[string]*Loaded
	states      map[string]LifecycleResult
	order       []string
	activated   bool
	closed      bool
	closeErr    error
}

func NewHost(registry *Registry) (*Host, error) {
	if registry == nil {
		return nil, errors.New("extensions: registry is required")
	}
	return &Host{
		registry: registry,
		plugins:  make(map[string]Plugin),
		loaded:   make(map[string]*Loaded),
		states:   make(map[string]LifecycleResult),
	}, nil
}

// Activate validates, orders, and loads one discovered plugin set. Individual
// failures are isolated and returned as results; calling Activate twice would
// make ownership ambiguous and is rejected.
func (h *Host) Activate(plugins []Plugin) ([]LifecycleResult, error) {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.stateMu.Lock()
	if h.closed {
		h.stateMu.Unlock()
		return nil, errHostClosed
	}
	if h.activated {
		h.stateMu.Unlock()
		return nil, errors.New("extensions: host is already activated")
	}
	h.activated = true

	resolved := resolve(plugins)
	for _, plugin := range resolved.plugins {
		h.plugins[plugin.ID] = clonePlugin(plugin)
		h.states[plugin.ID] = LifecycleResult{PluginID: plugin.ID, Phase: PluginAvailable}
	}
	for _, issue := range resolved.issues {
		h.states[issue.PluginID] = LifecycleResult{PluginID: issue.PluginID, Phase: PluginSkipped, Err: issue.Err}
	}
	for _, plugin := range resolved.order {
		h.order = append(h.order, plugin.ID)
	}
	order := slices.Clone(h.order)
	h.stateMu.Unlock()

	results := make([]LifecycleResult, 0, len(plugins))
	for _, issue := range resolved.issues {
		results = append(results, LifecycleResult{PluginID: issue.PluginID, Phase: PluginSkipped, Err: issue.Err})
	}
	for _, id := range order {
		results = append(results, h.load(id))
	}
	return results, nil
}

// Statuses returns lifecycle snapshots in resolved load order followed by
// structurally skipped plugins in stable identity order.
func (h *Host) Statuses() []Status {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	statuses := make([]Status, 0, len(h.plugins))
	seen := make(map[string]struct{}, len(h.plugins))
	for _, id := range h.order {
		statuses = append(statuses, h.statusLocked(id))
		seen[id] = struct{}{}
	}
	for id := range h.plugins {
		if _, exists := seen[id]; !exists {
			statuses = append(statuses, h.statusLocked(id))
		}
	}
	slices.SortStableFunc(statuses[len(h.order):], func(a, b Status) int { return strings.Compare(a.ID, b.ID) })
	return statuses
}

func (h *Host) statusLocked(id string) Status {
	plugin := h.plugins[id]
	state := h.states[id]
	detail := ""
	if state.Err != nil {
		detail = state.Err.Error()
	}
	return Status{
		ID: plugin.ID, Version: plugin.Version, APIVersion: plugin.APIVersion,
		Requires: slices.Clone(plugin.Requires), Capabilities: slices.Clone(plugin.Capabilities),
		Trusted: plugin.Trusted, Phase: state.Phase, Detail: detail,
	}
}

// Affected returns id and its transitive dependents in stable load order. An
// adapter uses it to retire provider-owned work before reload or unload.
func (h *Host) Affected(id string) ([]string, error) {
	if h == nil {
		return nil, errors.New("extensions: host is required")
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	if h.closed {
		return nil, errHostClosed
	}
	if _, exists := h.plugins[id]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	closure := h.dependentClosureLocked(id)
	affected := make([]string, 0, len(closure))
	for _, candidate := range h.order {
		if closure[candidate] {
			affected = append(affected, candidate)
			delete(closure, candidate)
		}
	}
	remaining := make([]string, 0, len(closure))
	for candidate := range closure {
		remaining = append(remaining, candidate)
	}
	slices.Sort(remaining)
	return append(affected, remaining...), nil
}

// Reload unloads a plugin and every transitive dependent, then reactivates the
// closure in dependency order so no dependent retains stale registrations.
func (h *Host) Reload(id string) ([]LifecycleResult, error) {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.stateMu.Lock()
	if h.closed {
		h.stateMu.Unlock()
		return nil, errHostClosed
	}
	if _, exists := h.plugins[id]; !exists {
		h.stateMu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	closure := h.dependentClosureLocked(id)
	order := slices.Clone(h.order)
	h.stateMu.Unlock()
	if err := h.unloadSet(closure); err != nil {
		return nil, fmt.Errorf("reload plugin %q: %w", id, err)
	}
	results := make([]LifecycleResult, 0, len(closure))
	for _, candidate := range order {
		if closure[candidate] {
			results = append(results, h.load(candidate))
		}
	}
	if len(results) == 0 {
		h.stateMu.RLock()
		state := h.states[id]
		h.stateMu.RUnlock()
		if state.Err == nil {
			state.Err = errors.New("extensions: plugin is not in the resolved dependency graph")
		}
		return []LifecycleResult{state}, nil
	}
	return results, nil
}

// Unload removes a plugin and its transitive dependents in reverse order.
func (h *Host) Unload(id string) error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.stateMu.Lock()
	if h.closed {
		h.stateMu.Unlock()
		return errHostClosed
	}
	if _, exists := h.plugins[id]; !exists {
		h.stateMu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	closure := h.dependentClosureLocked(id)
	h.stateMu.Unlock()
	return h.unloadSet(closure)
}

func (h *Host) dependentClosureLocked(id string) map[string]bool {
	closure := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for candidate, plugin := range h.plugins {
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

type pluginDisposal struct {
	id     string
	loaded *Loaded
}

func (h *Host) unloadSet(ids map[string]bool) error {
	h.stateMu.Lock()
	var disposables []pluginDisposal
	for _, id := range slices.Backward(h.order) {
		if !ids[id] {
			continue
		}
		if loaded := h.loaded[id]; loaded != nil {
			disposables = append(disposables, pluginDisposal{id: id, loaded: loaded})
			delete(h.loaded, id)
		}
		h.states[id] = LifecycleResult{PluginID: id, Phase: PluginAvailable}
	}
	h.stateMu.Unlock()
	var failures []error
	for _, disposable := range disposables {
		if err := disposable.loaded.Dispose(); err != nil {
			failure := fmt.Errorf("unload plugin %q: %w", disposable.id, err)
			failures = append(failures, failure)
			h.stateMu.Lock()
			h.states[disposable.id] = LifecycleResult{PluginID: disposable.id, Phase: PluginFailed, Err: failure}
			h.stateMu.Unlock()
		}
	}
	return errors.Join(failures...)
}

func (h *Host) load(id string) LifecycleResult {
	h.stateMu.Lock()
	plugin, exists := h.plugins[id]
	if !exists {
		h.stateMu.Unlock()
		return LifecycleResult{PluginID: id, Phase: PluginFailed, Err: fmt.Errorf("%w: %s", ErrPluginNotFound, id)}
	}
	if loaded := h.loaded[id]; loaded != nil {
		result := LifecycleResult{PluginID: id, Phase: PluginLoaded}
		h.states[id] = result
		h.stateMu.Unlock()
		return result
	}
	for _, dependency := range plugin.Requires {
		if h.loaded[dependency] == nil {
			result := LifecycleResult{
				PluginID: id, Phase: PluginSkipped,
				Err: fmt.Errorf("extensions: plugin %q requires inactive plugin %q", id, dependency),
			}
			h.states[id] = result
			h.stateMu.Unlock()
			return result
		}
	}
	h.stateMu.Unlock()
	loaded, err := Load(h.registry, plugin)
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	if err != nil {
		result := LifecycleResult{PluginID: id, Phase: PluginFailed, Err: err}
		h.states[id] = result
		return result
	}
	h.loaded[id] = loaded
	result := LifecycleResult{PluginID: id, Phase: PluginLoaded}
	h.states[id] = result
	return result
}

// Close unloads every plugin in reverse dependency order. It is idempotent and
// returns the same joined cleanup error to every caller.
func (h *Host) Close() error {
	if h == nil {
		return nil
	}
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.stateMu.Lock()
	if h.closed {
		err := h.closeErr
		h.stateMu.Unlock()
		return err
	}
	all := make(map[string]bool, len(h.plugins))
	for id := range h.plugins {
		all[id] = true
	}
	h.closed = true
	h.stateMu.Unlock()
	err := h.unloadSet(all)
	h.stateMu.Lock()
	h.closeErr = err
	h.stateMu.Unlock()
	return err
}
