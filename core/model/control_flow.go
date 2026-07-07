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

// Halt marks an error that should stop the current loop immediately.
//
// Implementations choose the continuation policy through Abort():
//   - true  means the run cannot continue and should fail.
//   - false means the run is suspended for human input and is expected to
//     resume.
type Halt interface {
	error

	// Abort reports the halt's intent. It is intentionally tiny so callers can
	// recognize control-flow signals without pulling in a loop-specific
	// package.
	Abort() bool
}

// IsControlFlowError reports whether err or one of its wrapped causes marks
// itself as expected control flow.
func IsControlFlowError(err error) bool {
	flow, ok := errors.AsType[ControlFlowError](err)
	return ok && flow.ControlFlow()
}
