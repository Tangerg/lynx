package run

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
)

// ErrInsufficientCapabilities reports that a caller cannot follow a Run because
// it lacks one or more capabilities the Run may exercise. Resume, subscribe, and
// pending-interrupt reads enforce the same refusal rather than silently reducing
// the Run's behavior.
var ErrInsufficientCapabilities = errors.New("run: caller capabilities are insufficient")

// InsufficientCapabilitiesError identifies the Run and the complete capability set
// the caller is missing.
type InsufficientCapabilitiesError struct {
	RunID   string
	Missing Capabilities
}

func (i *InsufficientCapabilitiesError) Error() string {
	return fmt.Sprintf("%s: run %q requires %s", ErrInsufficientCapabilities, i.RunID, i.Missing)
}

// Is lets callers branch on the category without discarding the missing set.
func (i *InsufficientCapabilitiesError) Is(target error) bool {
	return target == ErrInsufficientCapabilities
}

// Capabilities is the frozen set of optional behaviors a Run may exercise:
// whether it may create child Runs and which durable human waits it may produce.
//
// It sits beside [Limits] because admission fixes both values and every later
// segment obeys them. A continuation answers an interrupt; it does not renegotiate
// what the Run is allowed to do.
//
// Input boundaries translate negotiated caller capabilities into this value.
// The empty value is a valid minimal capability set, not an unknown value.
type Capabilities struct {
	ChildRuns      bool
	InterruptKinds []interrupt.Kind
}

// Clone returns an ownership-isolated copy.
func (c Capabilities) Clone() Capabilities {
	c.InterruptKinds = slices.Clone(c.InterruptKinds)
	return c
}

// Validate reports whether the capability set uses its one canonical
// representation. Admission normalizes once; later boundaries can compare the
// frozen value directly and fail closed on corruption.
func (c Capabilities) Validate() error {
	for index, kind := range c.InterruptKinds {
		if !kind.Valid() {
			return fmt.Errorf("run: run capability interrupt kind[%d] is unknown", index)
		}
		if index > 0 && c.InterruptKinds[index-1] >= kind {
			return errors.New("run: run capability interrupt kinds must be sorted without duplicates")
		}
	}
	return nil
}

// Equal reports semantic equality of two capability sets. It deliberately
// ignores order and duplicate spelling so callers validating an external value
// can explain a mismatch instead of accidentally comparing slice headers.
// Persisted and admitted values should additionally pass [Validate].
func (c Capabilities) Equal(other Capabilities) bool {
	left := c.Normalized()
	right := other.Normalized()
	return left.ChildRuns == right.ChildRuns && slices.Equal(left.InterruptKinds, right.InterruptKinds)
}

// Normalized returns the interrupt policy as a canonical set: sorted and without
// duplicates. Comparing or storing a set must not depend on how a caller happened
// to order it.
func (c Capabilities) Normalized() Capabilities {
	kinds := slices.Clone(c.InterruptKinds)
	slices.Sort(kinds)
	return Capabilities{
		ChildRuns:      c.ChildRuns,
		InterruptKinds: slices.Compact(kinds),
	}
}

// IsEmpty reports that the Run has no optional behavior enabled.
func (c Capabilities) IsEmpty() bool {
	return !c.ChildRuns && len(c.InterruptKinds) == 0
}

// MissingFrom returns every capability in c that caller lacks.
func (c Capabilities) MissingFrom(caller Capabilities) Capabilities {
	var missing Capabilities
	if c.ChildRuns && !caller.ChildRuns {
		missing.ChildRuns = true
	}
	for _, kind := range c.InterruptKinds {
		if !slices.Contains(caller.InterruptKinds, kind) {
			missing.InterruptKinds = append(missing.InterruptKinds, kind)
		}
	}
	return missing
}

// String names the enabled behaviors for an error explaining a refusal.
func (c Capabilities) String() string {
	parts := make([]string, 0, 1+len(c.InterruptKinds))
	if c.ChildRuns {
		parts = append(parts, "child runs")
	}
	for _, kind := range c.InterruptKinds {
		parts = append(parts, kind.String()+" interrupts")
	}
	if len(parts) == 0 {
		return "no optional capabilities"
	}
	return strings.Join(parts, ", ")
}
