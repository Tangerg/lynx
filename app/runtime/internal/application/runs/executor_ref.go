package runs

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
// the admitted session.
func (e ExecutorRef) ValidateFor(sessionID string) error {
	if strings.TrimSpace(e.SessionID) == "" || strings.TrimSpace(e.SessionID) != e.SessionID {
		return fmt.Errorf("%w: session ID must be non-empty without surrounding whitespace", ErrInvalidExecutorRef)
	}
	if strings.TrimSpace(e.ExecutorID) == "" || strings.TrimSpace(e.ExecutorID) != e.ExecutorID {
		return fmt.Errorf("%w: executor ID must be non-empty without surrounding whitespace", ErrInvalidExecutorRef)
	}
	if e.SessionID != sessionID {
		return fmt.Errorf("%w: executor session %q does not match admitted session %q", ErrInvalidExecutorRef, e.SessionID, sessionID)
	}
	return nil
}
