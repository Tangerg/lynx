package runtime

import (
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

	mu         sync.RWMutex
	named      map[string]any
	protected  map[string]struct{}
	objects    []any
	hidden     []any // intentionally a slice — Hide() must accept unhashable values too
	conditions map[string]bool
}

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
// Lookup("it", typeName).
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

// Bind implements dual-binding: the value lands at
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

func (b *inMemoryBlackboard) Condition(key string) (bool, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	value, ok := b.conditions[key]
	return value, ok
}

func (b *inMemoryBlackboard) Inspect(verbose bool) string {
	return core.InspectBlackboard(b, verbose)
}
