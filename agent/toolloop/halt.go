package toolloop

// Halt is the control-flow contract a tool error can carry.
//
// Halt belongs to the frozen legacy middleware path. New code should return
// [PauseError] or [AbortError] to a [Runner], which carry explicit state
// instead of overloading one boolean method. Halt remains here only while the
// legacy middleware still has real consumers; it is no longer owned by Core.
type Halt interface {
	error

	Abort() bool
}
