package runtime

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// BlackboardState is the ownership-isolated state required to restore
// a blackboard. Conditions are explicit boolean facts, Bindings and Objects
// preserve the named and insertion-ordered views, and Hidden preserves which
// historical values typed lookup must skip.
type BlackboardState struct {
	Bindings   core.Bindings
	Conditions map[string]bool
	Objects    []any
	Hidden     []any
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

// Both surfaces are found by type assertion, so a signature that drifts here
// does not fail the build — the capture simply stops being available. These
// assertions are what turns that into a compile error.
var (
	_ BlackboardSnapshotter = (*inMemoryBlackboard)(nil)
	_ BlackboardRestorer    = (*inMemoryBlackboard)(nil)
	_ BlackboardSnapshotter = (*declaredBlackboard)(nil)
	_ BlackboardRestorer    = (*declaredBlackboard)(nil)
)

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

func encodeBlackboardValues(codec core.SnapshotCodec, values []any) ([]core.TaggedValue, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded := make([]core.TaggedValue, len(values))
	for index, value := range values {
		tagged, err := codec.Encode(value)
		if err != nil {
			return nil, fmt.Errorf("values[%d]: %w", index, err)
		}
		encoded[index] = tagged
	}
	return encoded, nil
}

func decodeBlackboardValues(codec core.SnapshotCodec, values []core.TaggedValue) ([]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	decoded := make([]any, len(values))
	for index, tagged := range values {
		value, err := codec.Decode(tagged)
		if err != nil {
			return nil, fmt.Errorf("values[%d]: %w", index, err)
		}
		decoded[index] = value
	}
	return decoded, nil
}

// Snapshot implements [BlackboardSnapshotter].
func (b *inMemoryBlackboard) Snapshot() (BlackboardState, error) {
	b.mu.RLock()
	named := maps.Clone(b.named)
	storedObjects := slices.Clone(b.objects)
	storedHidden := slices.Clone(b.hidden)
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
	hidden := make([]any, len(storedHidden))
	for index, value := range storedHidden {
		hidden[index] = value.mustValue()
	}
	return BlackboardState{
		Bindings:   bindings,
		Conditions: conditions,
		Objects:    objects,
		Hidden:     hidden,
	}, nil
}

// Restore implements [BlackboardRestorer].
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
	hidden := make([]storedBlackboardValue, len(state.Hidden))
	for index, value := range state.Hidden {
		stored, err := storeBlackboardValue(value)
		if err != nil {
			return fmt.Errorf("restore hidden[%d]: %w", index, err)
		}
		hidden[index] = stored
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.named = named
	clear(b.conditions)
	maps.Copy(b.conditions, state.Conditions)
	b.objects = objects
	b.hidden = hidden
	return nil
}
