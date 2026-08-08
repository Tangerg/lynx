package run

// RunStatus is where a Run stands, coarsened to the three positions that decide
// what an observer does next: watch it, answer it, or read it. It is [RunState]
// projected — the three terminal reasons are one position, because "why did it
// end" is the [Outcome] and not the position.
//
// Reads filter on this domain value and durable records are keyed by it. Keeping
// one projection prevents observers and persistence from inventing independent
// spellings for the same three positions.
type RunStatus uint8

const (
	// StatusRunning — a segment is executing.
	StatusRunning RunStatus = iota
	// StatusWaiting — no segment is executing and the Run holds open interrupts,
	// so it is resumable and has no outcome.
	StatusWaiting
	// StatusFinished — no segment, no open interrupt, and a terminal outcome.
	StatusFinished
)

// Status projects s onto an observer's three positions. It is the ONLY such
// projection: the durable state column and the published status both derive from
// it, so one Run cannot read as running through one and finished through the
// other.
//
// It is exhaustive rather than defaulting. A state outside the machine is a
// programming error, and the cost of guessing is asymmetric: "running" is the one
// answer that makes an observer attach to a stream and a session keep its admission
// slot, so silently choosing it for a value nothing produced would turn a bug into
// a run nobody can finish.
func (s RunState) Status() RunStatus {
	switch s {
	case Running:
		return StatusRunning
	case Waiting:
		return StatusWaiting
	case Completed, Failed, Canceled:
		return StatusFinished
	default:
		panic("run: unknown state")
	}
}

// Valid reports whether s is one of the three positions. A decoded or
// caller-supplied value has to be checked before it selects rows.
func (s RunStatus) Valid() bool {
	return s == StatusRunning || s == StatusWaiting || s == StatusFinished
}

func (s RunStatus) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusWaiting:
		return "waiting"
	case StatusFinished:
		return "finished"
	default:
		return "unknown"
	}
}
