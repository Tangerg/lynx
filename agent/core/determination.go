// Package core defines the foundational types for the agent framework: three-valued
// logic, IO bindings, conditions, actions, goals, agents, blackboards, and the
// process-context plumbing that runtime layers consume.
package core

// Determination implements three-valued logic (Unknown / True / False). Unknown is
// the zero value so an unset condition reads as Unknown without explicit
// initialization — a property the GOAP planner relies on when pruning unreachable
// states.
type Determination int8

const (
	Unknown Determination = 0
	True    Determination = 1
	False   Determination = 2
)

func (d Determination) String() string {
	switch d {
	case True:
		return "true"
	case False:
		return "false"
	default:
		return "unknown"
	}
}

// And implements three-valued AND: False dominates; Unknown propagates otherwise.
func (d Determination) And(other Determination) Determination {
	if d == False || other == False {
		return False
	}
	if d == Unknown || other == Unknown {
		return Unknown
	}
	return True
}

// Or implements three-valued OR: True dominates; Unknown propagates otherwise.
func (d Determination) Or(other Determination) Determination {
	if d == True || other == True {
		return True
	}
	if d == Unknown || other == Unknown {
		return Unknown
	}
	return False
}

// Not flips True/False; Unknown stays Unknown.
func (d Determination) Not() Determination {
	switch d {
	case True:
		return False
	case False:
		return True
	default:
		return Unknown
	}
}

// FromBool lifts a Go bool into Determination.
func FromBool(b bool) Determination {
	if b {
		return True
	}
	return False
}
