package runtime

import (
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/google/uuid"
)

// inMemoryBlackboard is the default blackboard backed by maps and a slice.
// It is the only Blackboard implementation in the framework; production
// deployments that need persistence (Redis, Postgres, ...) write a custom
// implementation against the core.Blackboard interface.
//
// All public methods are safe for concurrent use. Reads use RLock, writes
// use Lock.
type inMemoryBlackboard struct {
	id string

	mu         sync.RWMutex
	named      map[string]any
	protected  map[string]struct{}
	objects    []any
	hidden     []any // intentionally a slice — Hide() must accept unhashable values too
	conditions map[string]bool
}

// newInMemoryBlackboard returns a fresh blackboard with a generated UUID id.
func newInMemoryBlackboard() *inMemoryBlackboard {
	return &inMemoryBlackboard{
		id:         uuid.NewString(),
		named:      map[string]any{},
		protected:  map[string]struct{}{},
		conditions: map[string]bool{},
	}
}

// Name identifies the in-memory blackboard implementation. The
// runtime treats Blackboard as an Extension; the registered prototype's
// Name() shows up in extension lists / debug output but is otherwise
// not load-bearing.
func (b *inMemoryBlackboard) Name() string { return "in-memory-blackboard" }

func (b *inMemoryBlackboard) ID() string { return b.id }

// Set stores under key AND appends to the ordered objects list. The
// dual-record is what makes "give me the latest of type T" work via
// GetValue("it", typeName).
func (b *inMemoryBlackboard) Set(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[key] = value
	b.objects = append(b.objects, value)
}

func (b *inMemoryBlackboard) Get(key string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.named[key]
	return v, ok
}

// GetValue resolves typed lookups:
//
//   - variable == "it" / empty: newest object whose stored type matches typeName.
//   - variable == "last_result":  newest object regardless of type.
//   - explicit name:             the value stored at that name, only if its type matches.
func (b *inMemoryBlackboard) GetValue(variable, typeName string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	switch variable {
	case "", core.DefaultBindingName:
		return b.findLatestByType(typeName)
	case core.LastResultBindingName:
		return b.findLatestVisible()
	}

	value, ok := b.named[variable]
	if !ok {
		return nil, false
	}
	if typeName != "" && !typeMatches(value, typeName) {
		return nil, false
	}
	return value, true
}

func (b *inMemoryBlackboard) HasValue(variable, typeName string) bool {
	_, ok := b.GetValue(variable, typeName)
	return ok
}

// findLatestByType walks the objects list in reverse, skipping hidden
// entries, returning the first match.
func (b *inMemoryBlackboard) findLatestByType(typeName string) (any, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		obj := b.objects[i]
		if b.isHidden(obj) {
			continue
		}
		if typeMatches(obj, typeName) {
			return obj, true
		}
	}
	return nil, false
}

// findLatestVisible returns the most-recently-added non-hidden object.
func (b *inMemoryBlackboard) findLatestVisible() (any, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		if !b.isHidden(b.objects[i]) {
			return b.objects[i], true
		}
	}
	return nil, false
}

// isHidden does a linear scan via DeepEqual; hidden lists are tiny in
// practice and we need DeepEqual because Go map keys can't accept
// unhashable struct types.
func (b *inMemoryBlackboard) isHidden(v any) bool {
	for _, h := range b.hidden {
		if reflect.DeepEqual(h, v) {
			return true
		}
	}
	return false
}

func (b *inMemoryBlackboard) AddObject(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.objects = append(b.objects, value)
}

func (b *inMemoryBlackboard) Objects() []any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return slices.Clone(b.objects)
}

// Bind implements the embabel 0.4 dual-binding behavior: the value lands at
// "it" AND at a type-derived key (UserInput → "user_input") so prompt
// templates can refer to it by either name.
func (b *inMemoryBlackboard) Bind(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[core.DefaultBindingName] = value
	if derivedKey := core.DerivedTypeKey(value); derivedKey != "" {
		b.named[derivedKey] = value
	}
	b.objects = append(b.objects, value)
}

func (b *inMemoryBlackboard) BindAll(m map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, value := range m {
		b.named[key] = value
		b.objects = append(b.objects, value)
	}
}

// BindProtected stores the value AND records the key as protected so a
// subsequent Spawn() carries it onto the child blackboard.
func (b *inMemoryBlackboard) BindProtected(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[key] = value
	b.protected[key] = struct{}{}
	b.objects = append(b.objects, value)
}

func (b *inMemoryBlackboard) Hide(target any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.hidden = append(b.hidden, target)
}

func (b *inMemoryBlackboard) SetCondition(key string, value bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.conditions[key] = value
}

func (b *inMemoryBlackboard) GetCondition(key string) (bool, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	value, ok := b.conditions[key]
	return value, ok
}

// Spawn produces a child blackboard. Default behavior copies named keys,
// protected entries, conditions, and the objects list — but NOT hidden
// markers (the child re-derives visibility on its own).
func (b *inMemoryBlackboard) Spawn() core.Blackboard {
	b.mu.RLock()
	defer b.mu.RUnlock()

	child := newInMemoryBlackboard()
	maps.Copy(child.named, b.named)
	maps.Copy(child.protected, b.protected)
	maps.Copy(child.conditions, b.conditions)
	child.objects = append(child.objects, b.objects...)
	return child
}

// Clear wipes the blackboard while preserving entries marked via
// BindProtected. Hidden markers and conditions are cleared.
func (b *inMemoryBlackboard) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	preserved := make(map[string]any, len(b.protected))
	for key := range b.protected {
		if value, ok := b.named[key]; ok {
			preserved[key] = value
		}
	}

	clear(b.named)
	maps.Copy(b.named, preserved)
	b.objects = b.objects[:0]
	b.hidden = b.hidden[:0]
	clear(b.conditions)
}

func (b *inMemoryBlackboard) InfoString(verbose bool) string {
	return core.InspectInfoString(b, verbose)
}

// typeMatches checks whether v matches typeName by walking the same rules
// IOBinding uses. Pointer types unwrap; sealed-interface hierarchies
// require explicit DomainType registration (handled by the determiner).
func typeMatches(v any, typeName string) bool {
	if typeName == "" {
		return true
	}
	if v == nil {
		return false
	}

	rt := reflect.TypeOf(v)
	for rt != nil {
		if core.TypeFullName(rt) == typeName {
			return true
		}
		if rt.Kind() != reflect.Pointer {
			break
		}
		rt = rt.Elem()
	}
	return false
}
