package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/toolloop"
)

// checkpointKey holds the target tool-loop checkpoint on the process
// blackboard. Process snapshots persist it together with the waiting action,
// so resume continues at the pending tool instead of replaying completed model
// and tool calls.
const checkpointKey = "lyra:toolloop:checkpoint"

// pendingInterruptKey is an in-tick handoff from the observed tool to runTurn.
// It is cleared before the action parks and is never part of durable state.
const pendingInterruptKey = "lyra:toolloop:pending-interrupt"

// Interrupt exposes the agent HITL primitive to runtime adapters.
func Interrupt[R any](ctx context.Context, key string, value any) (R, bool, error) {
	return hitl.Interrupt[R](ctx, key, value)
}

// IsInterrupt reports whether err carries the concrete agent HITL signal.
func IsInterrupt(err error) bool { return hitl.IsInterrupt(err) }

// HandleInterrupt parks pc on the concrete interrupt awaitable.
func HandleInterrupt(ctx context.Context, pc *core.ProcessContext, err error) (core.ActionStatus, bool) {
	return hitl.HandleInterrupt(ctx, pc, err)
}

type checkpointStore struct {
	bb core.Blackboard
}

func (s checkpointStore) Save(checkpoint *toolloop.Checkpoint) error {
	if checkpoint == nil {
		return errors.New("agentexec: tool-loop checkpoint is nil")
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("agentexec: invalid tool-loop checkpoint: %w", err)
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("agentexec: encode tool-loop checkpoint: %w", err)
	}
	s.bb.Set(checkpointKey, string(data))
	return nil
}

func (s checkpointStore) Load() (*toolloop.Checkpoint, bool, error) {
	value, ok := s.bb.Get(checkpointKey)
	if !ok {
		return nil, false, nil
	}
	data, ok := value.(string)
	if !ok {
		return nil, false, fmt.Errorf("agentexec: tool-loop checkpoint has type %T, want string", value)
	}
	if data == "" {
		return nil, false, nil
	}
	var checkpoint toolloop.Checkpoint
	if err := json.Unmarshal([]byte(data), &checkpoint); err != nil {
		return nil, false, fmt.Errorf("agentexec: decode tool-loop checkpoint: %w", err)
	}
	return &checkpoint, true, nil
}

func (s checkpointStore) Clear() { s.bb.Set(checkpointKey, "") }

func setPendingInterrupt(ctx context.Context, interrupt *hitl.InterruptError) error {
	process := core.ProcessFrom(ctx)
	if process == nil {
		return errors.New("agentexec: HITL interrupt has no process context")
	}
	process.Blackboard().Set(pendingInterruptKey, interrupt)
	return nil
}

func takePendingInterrupt(bb core.Blackboard) (*hitl.InterruptError, bool) {
	value, ok := bb.Get(pendingInterruptKey)
	if !ok {
		return nil, false
	}
	bb.Set(pendingInterruptKey, nil)
	interrupt, ok := value.(*hitl.InterruptError)
	return interrupt, ok && interrupt != nil
}

// ValidateInterruptSnapshot verifies that a waiting process snapshot contains
// a valid target tool-loop checkpoint.
func ValidateInterruptSnapshot(snapshot core.ProcessSnapshot) error {
	tag, ok := snapshot.Blackboard[checkpointKey]
	if !ok || len(tag.Value) == 0 {
		return errors.New("agentexec: interrupt snapshot has no tool-loop checkpoint")
	}
	var data string
	if err := json.Unmarshal(tag.Value, &data); err != nil {
		return fmt.Errorf("agentexec: decode snapshot checkpoint binding: %w", err)
	}
	if data == "" {
		return errors.New("agentexec: interrupt snapshot has an empty tool-loop checkpoint")
	}
	var checkpoint toolloop.Checkpoint
	if err := json.Unmarshal([]byte(data), &checkpoint); err != nil {
		return fmt.Errorf("agentexec: decode snapshot tool-loop checkpoint: %w", err)
	}
	return nil
}
