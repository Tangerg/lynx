package toolloop

// Halt is the control-flow contract a tool error can carry.
//
// When a tool returns an error implementing Halt, the loop stops and propagates
// the error unchanged instead of feeding it back to the model as a recoverable
// tool result. Ordinary errors remain recoverable. Observability classification
// is separate: a suspension that should not count as a failed model operation
// can also implement the shared control-flow marker from core/model.
type Halt interface {
	error

	// Abort reports the halt's intent:
	//   - true  means the run cannot continue and should fail.
	//   - false means the run is suspended for human input and is expected to
	//     resume.
	Abort() bool
}
