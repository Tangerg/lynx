package execution

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInsufficientCapabilities reports that a caller cannot follow a Run because
// it lacks one or more capabilities the Run may exercise. Resume, subscribe, and
// pending-interrupt reads enforce the same refusal rather than silently reducing
// the Run's behavior.
var ErrInsufficientCapabilities = errors.New("execution: caller capabilities are insufficient for the run")

// InsufficientCapabilities identifies the Run and the complete capability set
// the caller is missing.
type InsufficientCapabilities struct {
	RunID   string
	Missing RunCapabilities
}

func (e *InsufficientCapabilities) Error() string {
	return fmt.Sprintf("%s: run %q requires %s", ErrInsufficientCapabilities, e.RunID, e.Missing)
}

// Is lets callers branch on the category without discarding the missing set.
func (e *InsufficientCapabilities) Is(target error) bool {
	return target == ErrInsufficientCapabilities
}

// RunCapabilities is the frozen set of optional behaviors a Run may exercise:
// whether it may create child Runs and which durable human waits it may produce.
//
// It sits beside [RunLimits] because admission fixes both values and every later
// segment obeys them. A continuation answers an interrupt; it does not renegotiate
// what the Run is allowed to do.
//
// Input boundaries translate negotiated caller capabilities into this value.
// The empty value is a valid minimal capability set, not an unknown value.
type RunCapabilities struct {
	ChildRuns      bool
	InterruptKinds []InterruptKind
}

// Clone returns an ownership-isolated copy.
func (p RunCapabilities) Clone() RunCapabilities {
	p.InterruptKinds = slices.Clone(p.InterruptKinds)
	return p
}

// Validate reports whether the capability set uses its one canonical
// representation. Admission normalizes once; later boundaries can compare the
// frozen value directly and fail closed on corruption.
func (p RunCapabilities) Validate() error {
	for index, kind := range p.InterruptKinds {
		if !kind.Valid() {
			return fmt.Errorf("execution: run capability interrupt kind[%d] is unknown", index)
		}
		if index > 0 && p.InterruptKinds[index-1] >= kind {
			return errors.New("execution: run capability interrupt kinds must be sorted without duplicates")
		}
	}
	return nil
}

// Equal reports semantic equality of two capability sets. It deliberately
// ignores order and duplicate spelling so callers validating an external value
// can explain a mismatch instead of accidentally comparing slice headers.
// Persisted and admitted values should additionally pass [Validate].
func (p RunCapabilities) Equal(other RunCapabilities) bool {
	left := p.Normalized()
	right := other.Normalized()
	return left.ChildRuns == right.ChildRuns && slices.Equal(left.InterruptKinds, right.InterruptKinds)
}

// Normalized returns the interrupt policy as a canonical set: sorted and without
// duplicates. Comparing or storing a set must not depend on how a caller happened
// to order it.
func (p RunCapabilities) Normalized() RunCapabilities {
	kinds := slices.Clone(p.InterruptKinds)
	slices.Sort(kinds)
	return RunCapabilities{
		ChildRuns:      p.ChildRuns,
		InterruptKinds: slices.Compact(kinds),
	}
}

// IsEmpty reports that the Run has no optional behavior enabled.
func (p RunCapabilities) IsEmpty() bool {
	return !p.ChildRuns && len(p.InterruptKinds) == 0
}

// MissingFrom returns every capability in p that caller lacks.
func (p RunCapabilities) MissingFrom(caller RunCapabilities) RunCapabilities {
	var missing RunCapabilities
	if p.ChildRuns && !caller.ChildRuns {
		missing.ChildRuns = true
	}
	for _, kind := range p.InterruptKinds {
		if !slices.Contains(caller.InterruptKinds, kind) {
			missing.InterruptKinds = append(missing.InterruptKinds, kind)
		}
	}
	return missing
}

// String names the enabled behaviors for an error explaining a refusal.
func (p RunCapabilities) String() string {
	parts := make([]string, 0, 1+len(p.InterruptKinds))
	if p.ChildRuns {
		parts = append(parts, "child runs")
	}
	for _, kind := range p.InterruptKinds {
		parts = append(parts, kind.String()+" interrupts")
	}
	if len(parts) == 0 {
		return "no optional capabilities"
	}
	return strings.Join(parts, ", ")
}
