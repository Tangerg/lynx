package agent

import "errors"

// AcceptedMutationError reports that a runtime mutation returned successfully
// but its receipt violated the client contract. Receipt retains every usable
// identity from that response so callers can clean up the accepted mutation
// without replaying user intent under a new command identity.
type AcceptedMutationError struct {
	receipt SegmentStream
	cause   error
}

func NewAcceptedMutationError(receipt SegmentStream, cause error) error {
	if cause == nil {
		return nil
	}
	return &AcceptedMutationError{receipt: receipt, cause: cause}
}

func (a *AcceptedMutationError) Error() string { return a.cause.Error() }
func (a *AcceptedMutationError) Unwrap() error { return a.cause }

// AcceptedMutationReceipt extracts the partial receipt of a mutation known to
// have reached the runtime. False means the error came from the call itself and
// retains the ordinary uncertain/refused delivery semantics.
func AcceptedMutationReceipt(err error) (SegmentStream, bool) {
	accepted, ok := errors.AsType[*AcceptedMutationError](err)
	if !ok {
		return SegmentStream{}, false
	}
	return accepted.receipt, true
}
