package interaction

import (
	"errors"
	"fmt"
)

// ErrCommitted marks an interaction failure that occurred after observable
// model output or a tool result was committed. Re-running the enclosing action
// from its beginning could duplicate cost or side effects, so the runtime does
// not apply RetryPolicy retries to this error class.
var ErrCommitted = errors.New("interaction: committed boundary failed")

// CommittedError retains the boundary failure while also matching
// [ErrCommitted].
type CommittedError struct {
	Err error
}

func (e *CommittedError) Error() string {
	if e == nil || e.Err == nil {
		return ErrCommitted.Error()
	}
	return fmt.Sprintf("%s: %v", ErrCommitted, e.Err)
}

func (e *CommittedError) Unwrap() []error {
	if e == nil || e.Err == nil {
		return []error{ErrCommitted}
	}
	return []error{ErrCommitted, e.Err}
}

// Commit marks err as occurring after an externally observable interaction
// boundary. The process runtime uses this marker to prevent whole-action
// retries from duplicating model cost or tool side effects.
func Commit(err error) error {
	if err == nil || errors.Is(err, ErrCommitted) {
		return err
	}
	return &CommittedError{Err: err}
}
