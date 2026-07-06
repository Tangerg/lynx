package model

import "errors"

// ControlFlowError marks an error that represents expected control flow rather
// than a failed model operation. Observability code records it without setting
// error status or error.type metrics.
type ControlFlowError interface {
	error

	// ControlFlow reports whether this error should be treated as expected
	// control flow. Returning false lets wrapper types expose the method without
	// opting every instance into non-failure accounting.
	ControlFlow() bool
}

// IsControlFlowError reports whether err or one of its wrapped causes marks
// itself as expected control flow.
func IsControlFlowError(err error) bool {
	flow, ok := errors.AsType[ControlFlowError](err)
	return ok && flow.ControlFlow()
}
