package execution

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTurnRef reports an incomplete or cross-session turn identity.
var ErrInvalidTurnRef = errors.New("execution: invalid turn reference")

// TurnRef is the engine-neutral durable address of one executor turn. It is
// distinct from a logical Run: a resumed Run may attach another segment to the
// same durable Run while continuing to address its current executor turn here.
type TurnRef struct {
	SessionID string
	TurnID    string
}

// ValidateFor checks that the executor returned a complete identity bound to
// the session whose admission the application owns.
func (r TurnRef) ValidateFor(sessionID string) error {
	if strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.SessionID) != r.SessionID {
		return fmt.Errorf("%w: session ID must be non-empty without surrounding whitespace", ErrInvalidTurnRef)
	}
	if strings.TrimSpace(r.TurnID) == "" || strings.TrimSpace(r.TurnID) != r.TurnID {
		return fmt.Errorf("%w: turn ID must be non-empty without surrounding whitespace", ErrInvalidTurnRef)
	}
	if r.SessionID != sessionID {
		return fmt.Errorf("%w: turn session %q does not match admitted session %q", ErrInvalidTurnRef, r.SessionID, sessionID)
	}
	return nil
}
