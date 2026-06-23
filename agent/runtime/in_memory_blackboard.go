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

// Lookup resolves typed lookups:
//
//   - variable == "it" / empty: newest object whose stored type matches typeName.
//   - variable == "last_result":  newest object regardless of type.
//   - explicit name:             the value stored at that name, only if its type matches.
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
	if typeName != "" && !typeMatches(value, typeName) {
		return nil, false
	}
	return value, true
}

func (b *inMemoryBlackboard) HasValue(variable, typeName string) bool {
	_, ok := b.Lookup(variable, typeName)
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
// practice and DeepEqual is required because Go map keys can't accept
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

// Spawn produces a child blackboard inheriting the parent's full state: named
// keys, protected entries, conditions, the objects list — AND the hidden
// markers. Copying objects WITHOUT their hidden markers would un-hide them in
// the child: an object the parent deliberately hid (so actions stop discovering
// / re-binding it) would reappear via the child's findLatestByType, giving the
// child a different view of the same data than the parent had. Visibility is
// part of the inherited state, so it comes along. (Snapshot/Restore drop hidden
// instead — there it's a transient view filter with no portable wire form.)
func (b *inMemoryBlackboard) Spawn() core.Blackboard {
	b.mu.RLock()
	defer b.mu.RUnlock()

	child := newInMemoryBlackboard()
	maps.Copy(child.named, b.named)
	maps.Copy(child.protected, b.protected)
	maps.Copy(child.conditions, b.conditions)
	child.objects = append(child.objects, b.objects...)
	child.hidden = append(child.hidden, b.hidden...)
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

func (b *inMemoryBlackboard) Inspect(verbose bool) string {
	return core.InspectBlackboard(b, verbose)
}

// Snapshot implements [BlackboardSnapshotter] — returns shallow copies
// of the named bindings, conditions, and ordered objects list so the
// [AgentProcess.Snapshot] helper can persist them. Hidden + protected
// markers are deliberately omitted: protected re-applies naturally at
// restore time (a freshly-restored process behaves as though no
// reset has occurred), and Hide markers are a transient view filter
// that has no meaning outside the running process.
func (b *inMemoryBlackboard) Snapshot() (map[string]any, map[string]bool, []any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return maps.Clone(b.named), maps.Clone(b.conditions), slices.Clone(b.objects)
}

// Restore implements [BlackboardRestorer] — fills the blackboard
// from the values previously returned by [Snapshot]. Existing
// bindings are cleared first; partial restore is not supported.
// Protected / hidden markers are reset because they have no
// portable wire form.
func (b *inMemoryBlackboard) Restore(named map[string]any, conditions map[string]bool, objects []any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	clear(b.named)
	maps.Copy(b.named, named)
	clear(b.conditions)
	maps.Copy(b.conditions, conditions)
	b.objects = slices.Clone(objects)
	b.hidden = b.hidden[:0]
	clear(b.protected)
}

// typeMatches checks whether v matches typeName by walking the same rules
// IOBinding uses: pointer types unwrap, then the concrete type's full
// name is compared. Interface hierarchies are not walked — a binding
// matches the stored value's concrete type only.
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
