package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// Interrupt is the linear, typed human-input primitive. Its first invocation
// returns a SuspendedError carrying only JSON-safe state. After
// runtime.Engine.Resume records a schema-valid response, the action
// is re-entered and Interrupt decodes that response as R at the same call site.
func Interrupt[R any](ctx context.Context, key string, prompt any) (R, error) {
	var zero R
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return zero, errors.New("hitl.Interrupt: no process on context")
	}
	if key == "" {
		return zero, errors.New("hitl.Interrupt: key must not be empty")
	}

	if current := process.Suspension(); current != nil {
		switch {
		case current.ID == key && current.Responded():
			var response R
			if err := json.Unmarshal(current.Response, &response); err != nil {
				return zero, fmt.Errorf("hitl.Interrupt: decode response for %q: %w", key, err)
			}
			return response, nil
		case current.ID == key:
			return zero, &interaction.SuspendedError{Suspension: *current.Clone()}
		case !current.Responded():
			return zero, fmt.Errorf("%w: process is waiting on %q, not %q", interaction.ErrSuspensionConflict, current.ID, key)
		}
	}

	promptJSON, err := json.Marshal(prompt)
	if err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: encode prompt: %w", err)
	}
	schema, err := pkgjson.StringDefSchemaOf(zero)
	if err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: derive response schema: %w", err)
	}
	suspension := interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            key,
		Kind:          interaction.SuspensionHuman,
		Prompt:        promptJSON,
		ResumeSchema:  json.RawMessage(schema),
		CreatedAt:     time.Now(),
	}
	if err := suspension.Validate(); err != nil {
		return zero, fmt.Errorf("hitl.Interrupt: %w", err)
	}
	return zero, &interaction.SuspendedError{Suspension: suspension}
}

// IsInterrupt reports whether err carries a unified framework suspension.
func IsInterrupt(err error) bool { return errors.Is(err, interaction.ErrSuspended) }

// HandleInterrupt parks a suspension at an untyped action boundary. Typed
// actions perform the same translation automatically.
func HandleInterrupt(ctx context.Context, process *core.ProcessContext, err error) (core.ActionStatus, bool, error) {
	suspended, ok := errors.AsType[*interaction.SuspendedError](err)
	if !ok {
		return 0, false, nil
	}
	status, suspendErr := process.Suspend(ctx, suspended.Suspension)
	return status, true, suspendErr
}
