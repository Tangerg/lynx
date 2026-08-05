package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ErrInvalidControlFlow reports a malformed pause or abort signal.
var ErrInvalidControlFlow = errors.New("toolloop: invalid control flow")

// PauseError asks Runner to suspend before the current tool has produced side
// effects. ID must be stable across the resume attempt; Reason is suitable for
// an operator or approval UI.
type PauseError struct {
	ID             string
	Reason         string
	Prompt         json.RawMessage
	ResponseSchema json.RawMessage
}

func (e *PauseError) Error() string {
	if e == nil {
		return "toolloop: pause"
	}
	return fmt.Sprintf("toolloop: paused at %q: %s", e.ID, e.Reason)
}

func (e *PauseError) validate() error {
	if e == nil {
		return fmt.Errorf("%w: pause is nil", ErrInvalidControlFlow)
	}
	if err := interaction.ValidateID(e.ID); err != nil {
		return fmt.Errorf("%w: pause ID: %w", ErrInvalidControlFlow, err)
	}
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Errorf("%w: pause reason must not be empty", ErrInvalidControlFlow)
	}
	if !json.Valid(e.Prompt) {
		return fmt.Errorf("%w: pause prompt must be valid JSON", ErrInvalidControlFlow)
	}
	if !json.Valid(e.ResponseSchema) {
		return fmt.Errorf("%w: pause response schema must be valid JSON", ErrInvalidControlFlow)
	}
	return nil
}

// AbortError marks a tool failure that the model cannot recover from. Runner
// propagates it unchanged instead of turning it into an error ToolResult.
type AbortError struct {
	Err error
}

func (e *AbortError) Error() string {
	if e == nil || e.Err == nil {
		return "toolloop: abort"
	}
	return "toolloop: abort: " + e.Err.Error()
}

// Unwrap exposes the cause so errors.Is reaches whatever the caller aborted
// for. Unlike a pause, an abort ends the loop, and the reason it ended is the
// only thing left to report.
func (e *AbortError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *AbortError) validate() error {
	if e == nil || e.Err == nil {
		return fmt.Errorf("%w: abort requires a cause", ErrInvalidControlFlow)
	}
	return nil
}

type resumeContextKey struct{}

func withResume(ctx context.Context, resume Resume) context.Context {
	return context.WithValue(ctx, resumeContextKey{}, resume)
}

// ResumeFromContext returns the operator input attached to the currently
// resumed tool attempt. A tool normally checks this only after its approval
// guard has identified the same stable resume ID.
func ResumeFromContext(ctx context.Context) (Resume, bool) {
	if ctx == nil {
		return Resume{}, false
	}
	resume, ok := ctx.Value(resumeContextKey{}).(Resume)
	return resume, ok
}
