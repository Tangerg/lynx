package runtime

import (
	"fmt"
	"maps"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// BlackboardState is the ownership-isolated state required to restore
// a blackboard. Conditions are explicit boolean facts, while Bindings and
// Objects preserve the blackboard's named and insertion-ordered views.
type BlackboardState struct {
	Bindings   core.Bindings
	Conditions map[string]bool
	Objects    []any
}

// BlackboardSnapshotter is the optional capture surface a custom
// [core.Blackboard] implementation exposes so [Engine.SnapshotTree] can
// capture its full portable state.
type BlackboardSnapshotter interface {
	Snapshot() (BlackboardState, error)
}

// BlackboardRestorer is the optional restore surface.
type BlackboardRestorer interface {
	Restore(BlackboardState) error
}

func snapshotBlackboard(blackboard core.Blackboard) (state BlackboardState, err error) {
	snapshotter, ok := blackboard.(BlackboardSnapshotter)
	if !ok {
		return BlackboardState{}, fmt.Errorf("blackboard %T does not support snapshot capture", blackboard)
	}
	name, err := blackboardName(blackboard)
	if err != nil {
		return BlackboardState{}, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			state = BlackboardState{}
			err = panicerr.New(fmt.Sprintf("blackboard %q Snapshot panicked", name), recovered)
		}
	}()
	return snapshotter.Snapshot()
}

func restoreBlackboard(blackboard core.Blackboard, state BlackboardState) (err error) {
	restorer, ok := blackboard.(BlackboardRestorer)
	if !ok {
		return fmt.Errorf("blackboard %T does not support snapshot restore", blackboard)
	}
	name, err := blackboardName(blackboard)
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("blackboard %q Restore panicked", name), recovered)
		}
	}()
	return restorer.Restore(state)
}

// Snapshot implements [BlackboardSnapshotter]. Hide markers are deliberately
// omitted because they are a process-local view filter with no portable wire
// form.
func (b *inMemoryBlackboard) Snapshot() (BlackboardState, error) {
	b.mu.RLock()
	named := make(map[string]storedBlackboardValue, len(b.named))
	for key, value := range b.named {
		named[key] = value.clone()
	}
	storedObjects := make([]storedBlackboardValue, len(b.objects))
	for index, value := range b.objects {
		storedObjects[index] = value.clone()
	}
	conditions := maps.Clone(b.conditions)
	b.mu.RUnlock()

	var bindings core.Bindings
	for key, value := range named {
		bindings.Set(key, value.mustValue())
	}
	objects := make([]any, len(storedObjects))
	for index, value := range storedObjects {
		objects[index] = value.mustValue()
	}
	return BlackboardState{
		Bindings:   bindings,
		Conditions: conditions,
		Objects:    objects,
	}, nil
}

// Restore implements [BlackboardRestorer]. Existing bindings are cleared first;
// hidden markers are reset because they have no portable wire form.
func (b *inMemoryBlackboard) Restore(state BlackboardState) error {
	named := make(map[string]storedBlackboardValue, state.Bindings.Len())
	for key, value := range state.Bindings.All() {
		stored, err := storeBlackboardValue(value)
		if err != nil {
			return fmt.Errorf("restore blackboard[%q]: %w", key, err)
		}
		named[key] = stored
	}
	objects := make([]storedBlackboardValue, len(state.Objects))
	for index, value := range state.Objects {
		stored, err := storeBlackboardValue(value)
		if err != nil {
			return fmt.Errorf("restore objects[%d]: %w", index, err)
		}
		objects[index] = stored
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.named = named
	clear(b.conditions)
	maps.Copy(b.conditions, state.Conditions)
	b.objects = objects
	b.hidden = b.hidden[:0]
	return nil
}
