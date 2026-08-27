package interaction

import "errors"

var ErrHostFailure = errors.New("interaction: host failure")

type hostFailureError struct {
	cause error
}

func (h hostFailureError) Error() string { return h.cause.Error() }

func (h hostFailureError) Unwrap() error { return h.cause }

func (hostFailureError) Is(target error) bool { return target == ErrHostFailure }

// HostFailure marks cause as an Interaction-host failure. A nil cause remains
// nil, and an already marked error is returned unchanged.
func HostFailure(cause error) error {
	if cause == nil || errors.Is(cause, ErrHostFailure) {
		return cause
	}
	return hostFailureError{cause: cause}
}
