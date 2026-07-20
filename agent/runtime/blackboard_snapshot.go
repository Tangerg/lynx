package runtime

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// BlackboardState is the ownership-isolated state required to durably restore
// a blackboard. Conditions are explicit boolean facts, while Bindings and
// Objects preserve the blackboard's named and insertion-ordered views.
type BlackboardState struct {
	Bindings   core.Bindings
	Conditions map[string]bool
	Objects    []any
}

// BlackboardSnapshotter is the optional capture surface a custom
// [core.Blackboard] implementation exposes so [Process.Snapshot] can persist
// its full state.
type BlackboardSnapshotter interface {
	Snapshot() (BlackboardState, error)
}

// BlackboardRestorer is the optional restore surface.
type BlackboardRestorer interface {
	Restore(BlackboardState) error
}

// Snapshot implements [BlackboardSnapshotter]. Hidden + protected markers are
// deliberately omitted: protected re-applies naturally at restore time, and
// Hide markers are a transient view filter with no portable wire form.
func (b *inMemoryBlackboard) Snapshot() (BlackboardState, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var bindings core.Bindings
	for key, value := range b.named.All() {
		if _, transient := b.transientNamed[key]; !transient {
			bindings.Set(key, value)
		}
	}
	objects := make([]any, 0, len(b.objects))
	for i, value := range b.objects {
		if i < len(b.durableObjects) && b.durableObjects[i] {
			objects = append(objects, value)
		}
	}
	return BlackboardState{
		Bindings:   bindings,
		Conditions: maps.Clone(b.conditions),
		Objects:    objects,
	}, nil
}

// Restore implements [BlackboardRestorer]. Existing bindings are cleared first;
// protected / hidden markers are reset because they have no portable wire form.
func (b *inMemoryBlackboard) Restore(state BlackboardState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.named = core.Bindings{}
	for key, value := range state.Bindings.All() {
		b.named.Set(key, value)
	}
	clear(b.transientNamed)
	clear(b.conditions)
	maps.Copy(b.conditions, state.Conditions)
	b.objects = slices.Clone(state.Objects)
	b.durableObjects = make([]bool, len(state.Objects))
	for i := range b.durableObjects {
		b.durableObjects[i] = true
	}
	b.hidden = b.hidden[:0]
	clear(b.protected)
	return nil
}
