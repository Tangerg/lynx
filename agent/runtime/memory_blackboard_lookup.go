package runtime

import (
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
)

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
func typeMatches(v any, typeName string) bool {
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
