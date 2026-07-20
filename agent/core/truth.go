package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

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

// Valid reports whether t is one of the three defined truth values.
func (t Truth) Valid() bool {
	return t >= Unknown && t <= False
}

func (t Truth) String() string {
	switch t {
	case True:
		return "true"
	case False:
		return "false"
	case Unknown:
		return "unknown"
	default:
		return fmt.Sprintf("invalid_truth(%d)", t)
	}
}

// And implements three-valued AND: False dominates; Unknown propagates otherwise.
func (t Truth) And(other Truth) Truth {
	if !t.Valid() || !other.Valid() {
		return Unknown
	}
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
	if !t.Valid() || !other.Valid() {
		return Unknown
	}
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

// ConditionSet maps condition keys to their required or produced truth values.
// It is used by action preconditions, action effects, goals, and world-state
// transitions. A nil set is valid and means no conditions.
type ConditionSet map[string]Truth

// Validate verifies every condition key and truth value.
func (conditions ConditionSet) Validate() error {
	var problems []error
	for _, key := range slices.Sorted(maps.Keys(conditions)) {
		if err := validateConditionKey(key); err != nil {
			problems = append(problems, err)
		}
		if truth := conditions[key]; !truth.Valid() {
			problems = append(problems, fmt.Errorf("condition %q has invalid truth value %d", key, truth))
		}
	}
	return errors.Join(problems...)
}

func validateConditionKey(key string) error {
	if key == "" {
		return errors.New("condition key is empty")
	}
	if strings.TrimSpace(key) != key {
		return fmt.Errorf("condition key %q has surrounding whitespace", key)
	}
	return nil
}

// Satisfies reports whether state contains every required truth value.
func (state ConditionSet) Satisfies(required ConditionSet) bool {
	for key, truth := range required {
		if state[key] != truth {
			return false
		}
	}
	return true
}

// Unsatisfied returns the requirements not currently satisfied by state.
// A nil result means every requirement is satisfied.
func (state ConditionSet) Unsatisfied(required ConditionSet) ConditionSet {
	var missing ConditionSet
	for key, truth := range required {
		if state[key] == truth {
			continue
		}
		if missing == nil {
			missing = make(ConditionSet)
		}
		missing[key] = truth
	}
	return missing
}
