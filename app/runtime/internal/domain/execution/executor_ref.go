package execution

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidExecutorRef reports an incomplete or cross-session executor
// identity.
var ErrInvalidExecutorRef = errors.New("execution: invalid executor reference")

// ExecutorRef is the implementation-neutral durable address of the execution
// backing a Run. A resumed Run keeps this identity while opening a new Segment.
type ExecutorRef struct {
	SessionID  string
	ExecutorID string
}

// ValidateFor checks that the executor returned a complete identity bound to
// the session whose admission the application owns.
func (r ExecutorRef) ValidateFor(sessionID string) error {
	if strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.SessionID) != r.SessionID {
		return fmt.Errorf("%w: session ID must be non-empty without surrounding whitespace", ErrInvalidExecutorRef)
	}
	if strings.TrimSpace(r.ExecutorID) == "" || strings.TrimSpace(r.ExecutorID) != r.ExecutorID {
		return fmt.Errorf("%w: executor ID must be non-empty without surrounding whitespace", ErrInvalidExecutorRef)
	}
	if r.SessionID != sessionID {
		return fmt.Errorf("%w: executor session %q does not match admitted session %q", ErrInvalidExecutorRef, r.SessionID, sessionID)
	}
	return nil
}
