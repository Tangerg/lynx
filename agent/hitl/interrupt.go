package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/tool"
)

// Interrupt is the linear, typed human-input primitive. Its first invocation
// returns a SuspendedError carrying only JSON-safe state. After
// runtime.Engine.Respond records a schema-valid response, the action
// is re-entered and Interrupt decodes that response as R at the same call site.
func Interrupt[R any](ctx context.Context, id string, prompt any) (R, error) {
	var zero R
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return zero, errors.New("hitl.Interrupt: no process on context")
	}
	if id == "" {
		return zero, errors.New("hitl.Interrupt: ID must not be empty")
	}

	if current := process.Suspension(); current != nil {
		switch {
		case current.ID == id && current.Responded():
			var response R
			if err := json.Unmarshal(current.Response, &response); err != nil {
				return zero, fmt.Errorf("hitl.Interrupt: decode response for %q: %w", id, err)
			}
			return response, nil
		case current.ID == id:
			return zero, &interaction.SuspendedError{Suspension: *current.Clone()}
		case !current.Responded():
			return zero, fmt.Errorf("%w: process is waiting on %q, not %q", interaction.ErrSuspensionConflict, current.ID, id)
		}
	}

	promptJSON, err := json.Marshal(prompt)
	if err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: encode prompt: %w", err)
	}
	schema, err := tool.Schema[R]()
	if err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: derive response schema: %w", err)
	}
	suspension := interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             id,
		Prompt:         promptJSON,
		ResponseSchema: json.RawMessage(schema),
		CreatedAt:      time.Now(),
	}
	if err := suspension.Validate(); err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: %w", err)
	}
	return zero, &interaction.SuspendedError{Suspension: suspension}
}

// IsSuspended reports whether err carries a unified framework suspension.
func IsSuspended(err error) bool { return errors.Is(err, interaction.ErrSuspended) }

// HandleSuspension parks a suspension at an untyped action boundary. Typed
// actions perform the same translation automatically.
func HandleSuspension(ctx context.Context, process *core.ProcessContext, err error) (core.ActionStatus, bool, error) {
	suspended, ok := errors.AsType[*interaction.SuspendedError](err)
	if !ok {
		return 0, false, nil
	}
	status, suspendErr := process.Suspend(ctx, suspended.Suspension)
	return status, true, suspendErr
}
