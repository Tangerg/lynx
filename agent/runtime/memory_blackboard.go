package runtime

import (
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
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

	mu             sync.RWMutex
	named          map[string]any
	transientNamed map[string]struct{}
	protected      map[string]struct{}
	objects        []any
	durableObjects []bool
	hidden         []any // intentionally a slice — Hide() must accept unhashable values too
	conditions     map[string]bool
}

func newInMemoryBlackboard() *inMemoryBlackboard {
	return &inMemoryBlackboard{
		id:             uuid.NewString(),
		named:          map[string]any{},
		transientNamed: map[string]struct{}{},
		protected:      map[string]struct{}{},
		conditions:     map[string]bool{},
	}
}

// Name identifies the in-memory blackboard implementation. The
// runtime treats Blackboard as an Extension; the registered prototype's
// Name() shows up in extension lists / debug output but is otherwise
// not load-bearing.
func (b *inMemoryBlackboard) Name() string { return "in-memory-blackboard" }

func (b *inMemoryBlackboard) ID() string { return b.id }

// Store saves under key and appends to the ordered objects list. The
// dual-record is what makes "give me the latest of type T" work via
// Lookup("it", typeName).
func (b *inMemoryBlackboard) Store(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[key] = value
	delete(b.transientNamed, key)
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, true)
}

func (b *inMemoryBlackboard) StoreTransient(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.named[key] = value
	b.transientNamed[key] = struct{}{}
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, false)
}

func (b *inMemoryBlackboard) Load(key string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.named[key]
	return v, ok
}

func (b *inMemoryBlackboard) Add(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, true)
}

func (b *inMemoryBlackboard) AddTransient(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, false)
}

func (b *inMemoryBlackboard) Objects() []any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return slices.Clone(b.objects)
}

// Bind implements dual-binding: the value lands at
// "it" AND at a type-derived key (UserInput → "user_input") so prompt
// templates can refer to it by either name.
func (b *inMemoryBlackboard) Bind(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[core.DefaultBindingName] = value
	delete(b.transientNamed, core.DefaultBindingName)
	if derivedKey := core.TypeKey(value); derivedKey != "" {
		b.named[derivedKey] = value
		delete(b.transientNamed, derivedKey)
	}
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, true)
}

func (b *inMemoryBlackboard) BindTransient(value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.named[core.DefaultBindingName] = value
	b.transientNamed[core.DefaultBindingName] = struct{}{}
	if derivedKey := core.TypeKey(value); derivedKey != "" {
		b.named[derivedKey] = value
		b.transientNamed[derivedKey] = struct{}{}
	}
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, false)
}

func (b *inMemoryBlackboard) StoreAll(bindings core.Bindings) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, value := range bindings.All() {
		b.named[key] = value
		delete(b.transientNamed, key)
		b.objects = append(b.objects, value)
		b.durableObjects = append(b.durableObjects, true)
	}
}

// StoreProtected stores the value AND records the key as protected so a
// subsequent Clone() carries it onto the child blackboard.
func (b *inMemoryBlackboard) StoreProtected(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[key] = value
	delete(b.transientNamed, key)
	b.protected[key] = struct{}{}
	b.objects = append(b.objects, value)
	b.durableObjects = append(b.durableObjects, true)
}

func (b *inMemoryBlackboard) Hide(target any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.hidden = append(b.hidden, target)
}

func (b *inMemoryBlackboard) StoreCondition(key string, value bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.conditions[key] = value
}

func (b *inMemoryBlackboard) Condition(key string) (bool, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	value, ok := b.conditions[key]
	return value, ok
}

func (b *inMemoryBlackboard) Inspect(verbose bool) string {
	return core.FormatBlackboard(b, verbose)
}

// Clone produces a child blackboard inheriting the parent's full state: named
// keys, protected entries, conditions, the objects list, and the hidden
// markers. Visibility is part of the inherited state for live child processes.
func (b *inMemoryBlackboard) Clone() core.Blackboard {
	b.mu.RLock()
	defer b.mu.RUnlock()

	child := newInMemoryBlackboard()
	maps.Copy(child.named, b.named)
	maps.Copy(child.transientNamed, b.transientNamed)
	maps.Copy(child.protected, b.protected)
	maps.Copy(child.conditions, b.conditions)
	child.objects = append(child.objects, b.objects...)
	child.durableObjects = append(child.durableObjects, b.durableObjects...)
	child.hidden = append(child.hidden, b.hidden...)
	return child
}

// ClearWorkingState removes ordinary state while preserving protected entries.
func (b *inMemoryBlackboard) ClearWorkingState() {
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
	clear(b.transientNamed)
	b.objects = b.objects[:0]
	b.durableObjects = b.durableObjects[:0]
	b.hidden = b.hidden[:0]
	clear(b.conditions)
}

// Lookup resolves typed lookups:
//
//   - variable == "it" / empty: newest object whose stored type matches typeName.
//   - variable == "last_result": newest object regardless of type.
//   - explicit name: the value stored at that name, only if its type matches.
func (b *inMemoryBlackboard) Lookup(variable, typeName string) (any, bool) {
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
	if typeName != "" && !b.typeMatches(value, typeName) {
		return nil, false
	}
	return value, true
}

func (b *inMemoryBlackboard) HasValue(variable, typeName string) bool {
	_, ok := b.Lookup(variable, typeName)
	return ok
}

func (b *inMemoryBlackboard) findLatestByType(typeName string) (any, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		obj := b.objects[i]
		if b.isHidden(obj) {
			continue
		}
		if b.typeMatches(obj, typeName) {
			return obj, true
		}
	}
	return nil, false
}

func (b *inMemoryBlackboard) findLatestVisible() (any, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		if !b.isHidden(b.objects[i]) {
			return b.objects[i], true
		}
	}
	return nil, false
}

func (b *inMemoryBlackboard) isHidden(v any) bool {
	for _, h := range b.hidden {
		if reflect.DeepEqual(h, v) {
			return true
		}
	}
	return false
}

// typeMatches checks whether v matches typeName by walking the same rules
// Binding uses: pointer types unwrap, then the concrete type's full
// name is compared. Interface hierarchies are not walked; a binding matches
// the stored value's concrete type only.
func (b *inMemoryBlackboard) typeMatches(v any, typeName string) bool {
	if typeName == "" {
		return true
	}
	if v == nil {
		return false
	}

	rt := reflect.TypeOf(v)
	for rt != nil {
		if core.TypeNameOf(rt) == typeName {
			return true
		}
		if rt.Kind() != reflect.Pointer {
			break
		}
		rt = rt.Elem()
	}
	return false
}
