package runtime

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// declaredBlackboard gates every write on the codec that would have to restore
// it. Without it the two ends of the same invariant disagree: any blackboard
// accepts a value it can serialize, while the snapshot codec accepts only a
// type the Agent declared, so an undeclared write succeeded and left the whole
// process unsnapshottable — a failure that surfaced at the next checkpoint,
// arbitrarily far from the write that caused it.
//
// It decorates rather than being built into the in-memory blackboard because a
// process can run on a blackboard the engine never constructs:
// [core.ProcessOptions] hands one over already populated, and a registered
// prototype arrives through Clone. Only a wrapper covers all three.
type declaredBlackboard struct {
	core.Blackboard
	codec core.SnapshotCodec
}

// declareWrites wraps blackboard so its writes are checked against codec.
// Wrapping an already-wrapped blackboard replaces the codec rather than
// stacking, so a child process is gated by its own agent's declarations.
func declareWrites(blackboard core.Blackboard, codec core.SnapshotCodec) core.Blackboard {
	if valueIsNil(blackboard) {
		return blackboard
	}
	if declared, ok := blackboard.(*declaredBlackboard); ok {
		return &declaredBlackboard{Blackboard: declared.Blackboard, codec: codec}
	}
	return &declaredBlackboard{Blackboard: blackboard, codec: codec}
}

// Snapshot and Restore forward the optional capture surfaces the engine and any
// host wrapper discover by type assertion. A decorator that merely embedded the
// blackboard would hide them, silently turning a snapshottable process into one
// that cannot be captured; forwarding unconditionally keeps the assertion true
// and lets the delegate report its own absence, naming the type that lacks it.
func (b *declaredBlackboard) Snapshot() (BlackboardState, error) {
	return snapshotBlackboard(b.Blackboard)
}

func (b *declaredBlackboard) Restore(state BlackboardState) error {
	return restoreBlackboard(b.Blackboard, state)
}

func (b *declaredBlackboard) Store(key string, value any) error {
	if err := b.codec.Declares(value); err != nil {
		return fmt.Errorf("blackboard Store(%q): %w", key, err)
	}
	return b.Blackboard.Store(key, value)
}

func (b *declaredBlackboard) Add(value any) error {
	if err := b.codec.Declares(value); err != nil {
		return fmt.Errorf("blackboard Add: %w", err)
	}
	return b.Blackboard.Add(value)
}

func (b *declaredBlackboard) Bind(value any) error {
	if err := b.codec.Declares(value); err != nil {
		return fmt.Errorf("blackboard Bind: %w", err)
	}
	return b.Blackboard.Bind(value)
}

// StoreAll checks every binding before delegating, so a rejected value cannot
// leave the earlier ones committed.
func (b *declaredBlackboard) StoreAll(bindings core.Bindings) error {
	for key, value := range bindings.All() {
		if err := b.codec.Declares(value); err != nil {
			return fmt.Errorf("blackboard StoreAll(%q): %w", key, err)
		}
	}
	return b.Blackboard.StoreAll(bindings)
}

// Clone keeps the gate on the copy. A branch or child that inherited state
// under one codec must not be able to widen it by cloning.
func (b *declaredBlackboard) Clone() (core.Blackboard, error) {
	clone, err := b.Blackboard.Clone()
	if err != nil {
		return nil, err
	}
	if valueIsNil(clone) {
		return nil, fmt.Errorf("blackboard %T Clone returned nil", b.Blackboard)
	}
	return declareWrites(clone, b.codec), nil
}

// Hide is not gated: a hide marker names a value the blackboard already holds
// and never reaches a snapshot, which is why capture drops the markers.
