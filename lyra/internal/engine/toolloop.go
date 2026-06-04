package engine

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// toolLoopCheckpointKey is the blackboard key the per-turn tool-loop
// checkpoint lives under. There is one process (hence one blackboard) per
// turn, so a single name binding suffices; it rides the same in-memory
// blackboard across the suspend → ResumeProcess → re-tick cycle.
const toolLoopCheckpointKey = "lyra:toolloop:checkpoint"

// blackboardToolLoopStore is the [chat.ToolLoopStore] backed by a process
// blackboard. It turns the chat tool loop's HITL suspend into a true
// R-model: a gated tool's suspension checkpoints the in-flight tool round
// onto the blackboard, and the continuation run resumes from the pending
// (just-approved) call — NOT re-invoking the model for completed rounds,
// NOT re-executing already-run tools.
//
// In-process resume (the common case) re-ticks the same process against
// the same in-memory blackboard, so the checkpoint is present when the
// chat action body re-runs. Cross-restart resume rebuilds the blackboard
// from a JSON snapshot, where the message-typed checkpoint does not round
// trip; [blackboardToolLoopStore.Load] then returns false and the turn
// falls back to the legacy full re-run — acceptable, since restart-spanning
// resume is rare and already best-effort.
type blackboardToolLoopStore struct {
	bb core.Blackboard
}

// Load returns the checkpoint bound on the blackboard, or (nil, false)
// when none is present (fresh turn) or it was cleared.
func (s *blackboardToolLoopStore) Load(context.Context) (*chat.ToolLoopCheckpoint, bool) {
	v, ok := s.bb.Get(toolLoopCheckpointKey)
	if !ok {
		return nil, false
	}
	cp, ok := v.(*chat.ToolLoopCheckpoint)
	if !ok || cp == nil {
		return nil, false
	}
	return cp, true
}

// Save binds cp on the blackboard so the next re-tick's Load finds it.
func (s *blackboardToolLoopStore) Save(_ context.Context, cp *chat.ToolLoopCheckpoint) error {
	s.bb.Set(toolLoopCheckpointKey, cp)
	return nil
}

// Clear drops a consumed checkpoint by binding a typed nil — the
// blackboard has no delete, and Load treats nil as absent.
func (s *blackboardToolLoopStore) Clear(context.Context) error {
	s.bb.Set(toolLoopCheckpointKey, (*chat.ToolLoopCheckpoint)(nil))
	return nil
}
