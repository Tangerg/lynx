package core

// Truth implements three-valued logic (Unknown / True / False). Unknown is
// the zero value so an unset condition reads as Unknown without explicit
// initialization — a property the GOAP planner relies on when pruning unreachable
// states.
type Truth int8

const (
	Unknown Truth = 0
	True    Truth = 1
	False   Truth = 2
)

func (t Truth) String() string {
	switch t {
	case True:
		return "true"
	case False:
		return "false"
	default:
		return "unknown"
	}
}

// And implements three-valued AND: False dominates; Unknown propagates otherwise.
func (t Truth) And(other Truth) Truth {
	if t == False || other == False {
		return False
	}
	if t == Unknown || other == Unknown {
		return Unknown
	}
	return True
}

// Or implements three-valued OR: True dominates; Unknown propagates otherwise.
func (t Truth) Or(other Truth) Truth {
	if t == True || other == True {
		return True
	}
	if t == Unknown || other == Unknown {
		return Unknown
	}
	return False
}

// Not flips True/False; Unknown stays Unknown.
func (t Truth) Not() Truth {
	switch t {
	case True:
		return False
	case False:
		return True
	default:
		return Unknown
	}
}

// TruthOf lifts a Go bool into three-valued logic.
func TruthOf(value bool) Truth {
	if value {
		return True
	}
	return False
}
