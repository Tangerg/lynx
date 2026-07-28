package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
)

const inMemoryBlackboardName = "in-memory-blackboard"

// inMemoryBlackboard is the default blackboard backed by maps and a slice.
// It is the runtime's default Blackboard implementation. Hosts that need a
// different lifecycle provide an implementation through the core.Blackboard
// extension contract.
//
// All public methods are safe for concurrent use. Reads use RLock, writes
// use Lock.
type inMemoryBlackboard struct {
	id string

	mu         sync.RWMutex
	named      map[string]storedBlackboardValue
	objects    []storedBlackboardValue
	hidden     []storedBlackboardValue
	conditions map[string]bool
}

func newInMemoryBlackboard() *inMemoryBlackboard {
	return &inMemoryBlackboard{
		id:         uuid.NewString(),
		named:      map[string]storedBlackboardValue{},
		conditions: map[string]bool{},
	}
}

// storedBlackboardValue is the in-memory ownership boundary. Keeping the exact
// concrete type beside its JSON form lets every read reconstruct a fresh Go
// value without exposing the blackboard's state graph.
type storedBlackboardValue struct {
	typ  reflect.Type
	data []byte
}

func storeBlackboardValue(value any) (storedBlackboardValue, error) {
	stored := storedBlackboardValue{typ: reflect.TypeOf(value)}
	if err := requirePortableType(stored.typ); err != nil {
		return storedBlackboardValue{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return storedBlackboardValue{}, fmt.Errorf("encode %T: %w", value, err)
	}
	stored.data = slices.Clone(data)
	if _, err := stored.value(); err != nil {
		return storedBlackboardValue{}, fmt.Errorf("decode %T: %w", value, err)
	}
	return stored, nil
}

func (v storedBlackboardValue) value() (any, error) {
	if v.typ == nil {
		return nil, nil
	}
	target := reflect.New(v.typ)
	if err := json.Unmarshal(v.data, target.Interface()); err != nil {
		return nil, err
	}
	return target.Elem().Interface(), nil
}

func (v storedBlackboardValue) mustValue() any {
	value, err := v.value()
	if err != nil {
		panic(fmt.Sprintf("agent runtime: corrupt in-memory blackboard value %s: %v", v.typ, err))
	}
	return value
}

func (v storedBlackboardValue) clone() storedBlackboardValue {
	return storedBlackboardValue{typ: v.typ, data: slices.Clone(v.data)}
}

// Name identifies the in-memory blackboard implementation. The
// runtime treats Blackboard as an Extension; the registered prototype's
// Name() shows up in extension lists / debug output but is otherwise
// not load-bearing.
func (b *inMemoryBlackboard) Name() string { return inMemoryBlackboardName }

func (b *inMemoryBlackboard) ID() string { return b.id }

// Store saves under key and appends to the ordered objects list. The
// dual-record is what makes "give me the latest of type T" work via
// Lookup("it", typeName).
func (b *inMemoryBlackboard) Store(key string, value any) error {
	stored, err := storeBlackboardValue(value)
	if err != nil {
		return fmt.Errorf("blackboard Store(%q): %w", key, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[key] = stored
	b.objects = append(b.objects, stored)
	return nil
}

func (b *inMemoryBlackboard) Load(key string) (any, bool) {
	b.mu.RLock()
	value, ok := b.named[key]
	value = value.clone()
	b.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return value.mustValue(), true
}

func (b *inMemoryBlackboard) Add(value any) error {
	stored, err := storeBlackboardValue(value)
	if err != nil {
		return fmt.Errorf("blackboard Add: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.objects = append(b.objects, stored)
	return nil
}

func (b *inMemoryBlackboard) Objects() []any {
	b.mu.RLock()
	stored := make([]storedBlackboardValue, len(b.objects))
	for index, value := range b.objects {
		stored[index] = value.clone()
	}
	b.mu.RUnlock()

	objects := make([]any, len(stored))
	for index, value := range stored {
		objects[index] = value.mustValue()
	}
	return objects
}

// Bind implements dual-binding: the value lands at
// "it" AND at a type-derived key (UserInput → "user_input") so prompt
// templates can refer to it by either name.
func (b *inMemoryBlackboard) Bind(value any) error {
	stored, err := storeBlackboardValue(value)
	if err != nil {
		return fmt.Errorf("blackboard Bind: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named[core.DefaultBindingName] = stored
	if derivedKey := core.TypeKey(value); derivedKey != "" {
		b.named[derivedKey] = stored
	}
	b.objects = append(b.objects, stored)
	return nil
}

func (b *inMemoryBlackboard) StoreAll(bindings core.Bindings) error {
	type namedValue struct {
		key   string
		value storedBlackboardValue
	}
	values := make([]namedValue, 0, bindings.Len())
	for key, value := range bindings.All() {
		stored, err := storeBlackboardValue(value)
		if err != nil {
			return fmt.Errorf("blackboard StoreAll(%q): %w", key, err)
		}
		values = append(values, namedValue{key: key, value: stored})
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, value := range values {
		b.named[value.key] = value.value
		b.objects = append(b.objects, value.value)
	}
	return nil
}

func (b *inMemoryBlackboard) Hide(target any) error {
	stored, err := storeBlackboardValue(target)
	if err != nil {
		return fmt.Errorf("blackboard Hide: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.hidden = append(b.hidden, stored)
	return nil
}

func (b *inMemoryBlackboard) StoreCondition(key string, value bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.conditions[key] = value
	return nil
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
// keys, conditions, the objects list, and the hidden markers. Visibility is
// part of the inherited state for live child processes.
func (b *inMemoryBlackboard) Clone() (core.Blackboard, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	child := newInMemoryBlackboard()
	maps.Copy(child.conditions, b.conditions)
	for key, value := range b.named {
		child.named[key] = value.clone()
	}
	child.objects = make([]storedBlackboardValue, len(b.objects))
	for index, value := range b.objects {
		child.objects[index] = value.clone()
	}
	child.hidden = make([]storedBlackboardValue, len(b.hidden))
	for index, value := range b.hidden {
		child.hidden[index] = value.clone()
	}
	return child, nil
}

// ClearWorkingState removes all planner/action working state.
func (b *inMemoryBlackboard) ClearWorkingState() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	clear(b.named)
	b.objects = b.objects[:0]
	b.hidden = b.hidden[:0]
	clear(b.conditions)
	return nil
}

// Lookup resolves typed lookups:
//
//   - variable == "it" / empty: newest object whose stored type matches typeName.
//   - variable == "last_result": newest object regardless of type.
//   - explicit name: the value stored at that name, only if its type matches.
func (b *inMemoryBlackboard) Lookup(variable, typeName string) (any, bool) {
	b.mu.RLock()
	value, ok := b.lookup(variable, typeName)
	value = value.clone()
	b.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return value.mustValue(), true
}

func (b *inMemoryBlackboard) lookup(variable, typeName string) (storedBlackboardValue, bool) {
	switch variable {
	case "", core.DefaultBindingName:
		return b.findLatestByType(typeName)
	case core.LastResultBindingName:
		return b.findLatestVisible()
	}

	value, ok := b.named[variable]
	if !ok {
		return storedBlackboardValue{}, false
	}
	if typeName != "" && !b.typeMatches(value, typeName) {
		return storedBlackboardValue{}, false
	}
	return value, true
}

func (b *inMemoryBlackboard) HasValue(variable, typeName string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.lookup(variable, typeName)
	return ok
}

func (b *inMemoryBlackboard) findLatestByType(typeName string) (storedBlackboardValue, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		obj := b.objects[i]
		if b.isHidden(obj) {
			continue
		}
		if b.typeMatches(obj, typeName) {
			return obj, true
		}
	}
	return storedBlackboardValue{}, false
}

func (b *inMemoryBlackboard) findLatestVisible() (storedBlackboardValue, bool) {
	for i := len(b.objects) - 1; i >= 0; i-- {
		if !b.isHidden(b.objects[i]) {
			return b.objects[i], true
		}
	}
	return storedBlackboardValue{}, false
}

func (b *inMemoryBlackboard) isHidden(v storedBlackboardValue) bool {
	for _, h := range b.hidden {
		if h.typ == v.typ && bytes.Equal(h.data, v.data) {
			return true
		}
	}
	return false
}

// typeMatches checks whether v matches typeName by walking the same rules
// Binding uses: pointer types unwrap, then the concrete type's full
// name is compared. Interface hierarchies are not walked; a binding matches
// the stored value's concrete type only.
func (b *inMemoryBlackboard) typeMatches(v storedBlackboardValue, typeName string) bool {
	if typeName == "" {
		return true
	}
	if v.typ == nil {
		return false
	}

	rt := v.typ
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
