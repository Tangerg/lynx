// Package extensions provides the application's typed extension substrate.
//
// A point is a typed address, a registry owns contributions, and a plugin owns
// every contribution it installs. The registry is deliberately independent of
// Cobra, oolong, and the runtime so each outer adapter can expose only the
// extension points it actually consumes.
package extensions

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
)

var errScopeClosed = errors.New("extensions: plugin scope is closed")

// Keying says whether a point has one contribution per stable key or permits
// multiple independent contributions.
type Keying string

const (
	Keyed Keying = "keyed"
	Multi Keying = "multi"
)

// Valid reports whether keying names one supported contribution cardinality.
func (k Keying) Valid() bool { return k == Keyed || k == Multi }

// Capability names one permission a plugin may use. Capabilities are attached
// to extension points, so policy is enforced at the operation rather than by
// trusting each plugin to police itself.
type Capability string

// Point is a typed handle shared by contributors and consumers. It holds no
// state; Registry is the single source of truth.
type Point[T any] struct {
	id         string
	keying     Keying
	capability Capability
	keyOf      func(T) string
}

// NewKeyedPoint defines a point with one contribution per non-empty key.
func NewKeyedPoint[T any](id string, keyOf func(T) string) Point[T] {
	return Point[T]{id: id, keying: Keyed, keyOf: keyOf}
}

// NewMultiPoint defines a point where every contribution coexists.
func NewMultiPoint[T any](id string) Point[T] {
	return Point[T]{id: id, keying: Multi}
}

// NewCapabilityMultiPoint defines a protected point where ordered contributions
// coexist.
func NewCapabilityMultiPoint[T any](id string, capability Capability) Point[T] {
	return Point[T]{id: id, keying: Multi, capability: capability}
}

// NewCapabilityKeyedPoint defines a protected keyed point.
func NewCapabilityKeyedPoint[T any](id string, capability Capability, keyOf func(T) string) Point[T] {
	return Point[T]{id: id, keying: Keyed, capability: capability, keyOf: keyOf}
}

// Capability reports the permission required to contribute to this point.
func (p Point[T]) Capability() Capability { return p.capability }

// Registry stores all contributions. Its zero value is ready and safe for
// concurrent setup, lookup, and disposal.
type Registry struct {
	mu      sync.RWMutex
	points  map[string]pointState
	plugins map[string]uint64
	next    uint64
}

type pointState struct {
	typeOf  reflect.Type
	keying  Keying
	entries map[string]entry
}

type entry struct {
	plugin string
	order  int
	seq    uint64
	value  any
}

// Contribution configures one registration.
type Contribution struct {
	// Key is required only when a keyed point has no keyOf function.
	Key string
	// Order sorts lower values first; registration order breaks ties.
	Order int
}

// Disposable releases an owned registration. Dispose is idempotent.
type Disposable interface {
	Dispose() error
}

type disposal struct {
	once sync.Once
	do   func() error
	err  error
}

func (d *disposal) Dispose() error {
	if d == nil {
		return nil
	}
	d.once.Do(func() { d.err = d.do() })
	return d.err
}

// Scope is the capability a plugin receives during setup. It owns every
// disposable created through Contribute, enabling exact rollback and unload.
// The scope is sealed when setup returns; plugins cannot attach ownership to a
// completed or rolling-back installation transaction.
type Scope struct {
	mu           sync.Mutex
	plugin       string
	registry     *Registry
	capabilities map[Capability]struct{}
	disposables  []Disposable
	open         bool
}

// OnDispose binds a plugin-owned side effect to rollback, unload, and reload.
// Cleanups run in reverse registration order.
func (s *Scope) OnDispose(cleanup func() error) error {
	if s == nil || s.registry == nil {
		return errors.New("extensions: plugin scope is required")
	}
	if cleanup == nil {
		return errors.New("extensions: cleanup is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.open {
		return errScopeClosed
	}
	s.disposables = append(s.disposables, &disposal{do: cleanup})
	return nil
}

func (s *Scope) seal() []Disposable {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = false
	return slices.Clone(s.disposables)
}

// Plugin is one cohesive set of contributions.
type Plugin struct {
	ID           string
	Version      string
	APIVersion   int
	Requires     []string
	Capabilities []Capability
	// Trusted permits an in-process composition root to omit a capability list.
	// Never derive it from a sideloaded manifest or another external source.
	Trusted bool
	Setup   func(*Scope) error
}

// Loaded is a successfully initialized plugin. Dispose unloads it in reverse
// registration order.
type Loaded struct {
	once        sync.Once
	disposables []Disposable
	err         error
}

// Dispose unloads a plugin. Every cleanup runs even when another returns an
// error or panics; the joined result is stable across repeated calls.
func (l *Loaded) Dispose() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		var failures []error
		for _, disposable := range slices.Backward(l.disposables) {
			if err := disposeSafely(disposable); err != nil {
				failures = append(failures, err)
			}
		}
		l.err = errors.Join(failures...)
	})
	return l.err
}

func disposeSafely(disposable Disposable) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin cleanup panicked: %v", recovered)
		}
	}()
	if disposable == nil {
		return nil
	}
	return disposable.Dispose()
}

// Load validates and initializes one plugin. Setup is transactional: a failure
// rolls back everything contributed before the error.
func Load(registry *Registry, plugin Plugin) (*Loaded, error) {
	if registry == nil {
		return nil, errors.New("extensions: registry is required")
	}
	if err := ValidateManifest(plugin); err != nil {
		return nil, err
	}
	release, err := registry.claim(plugin.ID)
	if err != nil {
		return nil, err
	}
	scope := &Scope{
		plugin: plugin.ID, registry: registry,
		capabilities: capabilitySet(plugin),
		disposables:  []Disposable{release},
		open:         true,
	}
	setupErr := setupSafely(plugin.Setup, scope)
	disposables := scope.seal()
	if setupErr != nil {
		loaded := &Loaded{disposables: disposables}
		if rollbackErr := loaded.Dispose(); rollbackErr != nil {
			setupErr = errors.Join(setupErr, fmt.Errorf("rollback plugin %q: %w", plugin.ID, rollbackErr))
		}
		return nil, fmt.Errorf("load plugin %q: %w", plugin.ID, setupErr)
	}
	return &Loaded{disposables: disposables}, nil
}

func setupSafely(setup func(*Scope) error, scope *Scope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin setup panicked: %v", recovered)
		}
	}()
	return setup(scope)
}

func capabilitySet(plugin Plugin) map[Capability]struct{} {
	if plugin.Trusted && plugin.Capabilities == nil {
		return nil
	}
	out := make(map[Capability]struct{}, len(plugin.Capabilities))
	for _, capability := range plugin.Capabilities {
		out[capability] = struct{}{}
	}
	return out
}

func (r *Registry) claim(plugin string) (Disposable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.plugins == nil {
		r.plugins = make(map[string]uint64)
	}
	if _, exists := r.plugins[plugin]; exists {
		return nil, fmt.Errorf("extensions: plugin %q is already loaded", plugin)
	}
	r.next++
	token := r.next
	r.plugins[plugin] = token
	return &disposal{do: func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.plugins[plugin] == token {
			delete(r.plugins, plugin)
		}
		return nil
	}}, nil
}

// Contribute adds a value to a point and makes its lifetime belong to s.
func (s *Scope) Contribute[T any](point Point[T], value T, options Contribution) (Disposable, error) {
	if s == nil || s.registry == nil {
		return nil, errors.New("extensions: plugin scope is required")
	}
	s.mu.Lock()
	err := s.validateContribution(point)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// keyOf belongs to the point owner and may execute arbitrary code. Run it
	// without the scope lock, then recheck the transaction before committing.
	key, err := point.contributionKey(value, options.Key)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if validateContributionErr := s.validateContribution(point); validateContributionErr != nil {
		return nil, validateContributionErr
	}
	key, sequence, err := s.registry.insertContribution(s.plugin, point, key, value, options.Order)
	if err != nil {
		return nil, err
	}
	d := &disposal{do: func() error {
		s.registry.removeContribution(point.id, key, sequence)
		return nil
	}}
	s.disposables = append(s.disposables, d)
	return d, nil
}

func (s *Scope) validateContribution[T any](point Point[T]) error {
	if !s.open {
		return errScopeClosed
	}
	if strings.TrimSpace(point.id) == "" {
		return errors.New("extensions: point id is required")
	}
	if !point.keying.Valid() {
		return fmt.Errorf("extensions: point %q has invalid keying %q", point.id, point.keying)
	}
	if point.capability == "" || s.capabilities == nil {
		return nil
	}
	if _, allowed := s.capabilities[point.capability]; !allowed {
		return fmt.Errorf("extensions: plugin %q needs capability %q to contribute to point %q", s.plugin, point.capability, point.id)
	}
	return nil
}

func (p Point[T]) contributionKey(value T, configured string) (string, error) {
	key := strings.TrimSpace(configured)
	if p.keying == Keyed && key == "" && p.keyOf != nil {
		key = strings.TrimSpace(p.keyOf(value))
	}
	if p.keying == Keyed && key == "" {
		return "", fmt.Errorf("extensions: keyed point %q requires a stable key", p.id)
	}
	return key, nil
}

func (r *Registry) insertContribution[T any](
	plugin string,
	point Point[T],
	key string,
	value T,
	order int,
) (string, uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.points == nil {
		r.points = make(map[string]pointState)
	}
	state, err := r.pointStateFor(point)
	if err != nil {
		return "", 0, err
	}
	r.next++
	sequence := r.next
	if point.keying == Multi {
		key = fmt.Sprintf("%s:%d", plugin, sequence)
	}
	if previous, duplicate := state.entries[key]; duplicate {
		return "", 0, fmt.Errorf("extensions: plugin %q cannot contribute key %q to point %q; owned by %q",
			plugin, key, point.id, previous.plugin)
	}
	state.entries[key] = entry{plugin: plugin, order: order, seq: sequence, value: value}
	r.points[point.id] = state
	return key, sequence, nil
}

func (r *Registry) pointStateFor[T any](point Point[T]) (pointState, error) {
	wantType := reflect.TypeFor[T]()
	state, exists := r.points[point.id]
	if exists && (state.typeOf != wantType || state.keying != point.keying) {
		return pointState{}, fmt.Errorf("extensions: point %q was defined with an incompatible type or keying", point.id)
	}
	if !exists {
		state = pointState{typeOf: wantType, keying: point.keying, entries: make(map[string]entry)}
	}
	return state, nil
}

func (r *Registry) removeContribution(pointID, key string, sequence uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.points[pointID]
	if !ok {
		return
	}
	current, ok := state.entries[key]
	if !ok || current.seq != sequence {
		return
	}
	delete(state.entries, key)
	r.points[pointID] = state
}

// OwnedValue keeps contribution ownership attached to a typed value so an
// adapter can cancel in-flight work before unloading its provider.
type OwnedValue[T any] struct {
	PluginID string
	Value    T
}

// OwnedValues returns a typed, stable snapshot ordered by Contribution.Order
// and then by registration order.
func (r *Registry) OwnedValues[T any](point Point[T]) []OwnedValue[T] {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	state, ok := r.points[point.id]
	if !ok || state.typeOf != reflect.TypeFor[T]() || state.keying != point.keying {
		r.mu.RUnlock()
		return nil
	}
	entries := make([]entry, 0, len(state.entries))
	for _, item := range state.entries {
		entries = append(entries, item)
	}
	r.mu.RUnlock()

	slices.SortStableFunc(entries, func(a, b entry) int {
		if order := cmp.Compare(a.order, b.order); order != 0 {
			return order
		}
		return cmp.Compare(a.seq, b.seq)
	})
	out := make([]OwnedValue[T], 0, len(entries))
	for _, item := range entries {
		out = append(out, OwnedValue[T]{PluginID: item.plugin, Value: item.value.(T)})
	}
	return out
}

// Values is OwnedValues with ownership metadata intentionally projected away.
func (r *Registry) Values[T any](point Point[T]) []T {
	owned := r.OwnedValues(point)
	out := make([]T, 0, len(owned))
	for _, item := range owned {
		out = append(out, item.Value)
	}
	return out
}
