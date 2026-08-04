package execution

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrProfileNotCovered reports that a caller cannot follow a Run: the Run's frozen
// profile names capabilities the caller did not declare. It lives with the profile
// rather than with either use case, because both the continuation path and the
// waiting-set read enforce the same rule, and one rule has one sentinel.
var ErrProfileNotCovered = errors.New("execution: caller does not cover the run's protocol profile")

// ProfileNotCovered is the refusal WITH its gap: which Run, and exactly what the
// caller would have to declare to follow it.
//
// The gap is a profile because that is what it is — a pair of sets — and because the
// caller has to be told all of it at once: a caller fixing one missing capability at
// a time never reaches a request that succeeds.
type ProfileNotCovered struct {
	RunID string
	Gap   RunProtocolProfile
}

func (e *ProfileNotCovered) Error() string {
	return fmt.Sprintf("%s: run %q publishes %s", ErrProfileNotCovered, e.RunID, e.Gap)
}

// Is answers to the sentinel, so a reader that only branches on "not covered" keeps
// working and only a reader that needs the gap asks for the type.
func (e *ProfileNotCovered) Is(target error) bool { return target == ErrProfileNotCovered }

// RunProtocolProfile is the protocol contract a Run is admitted under: which
// negotiated capabilities change what the Run publishes, and which durable human
// waits it may produce.
//
// It sits beside [RunLimits] because it is the same kind of fact — policy the
// admission fixes and every later segment obeys — and for the same reason: a
// continuation answers an interrupt, it does not renegotiate the terms the Run was
// accepted under. Limits bound what a Run may spend; the profile bounds what it
// may PUBLISH, and a Run whose contract changed mid-flight would hand its
// subscriber a stream it had not agreed to follow.
//
// ChildRuns and InterruptKinds are application semantics, not wire vocabulary.
// Delivery translates features.subagents into ChildRuns at admission and back
// when presenting the profile. Keeping the opaque capability key here would make
// the application interpret a transport term; keeping both would create two
// authors for the same policy.
//
// The empty profile is not "unset" — it is the Minimal Profile: a Run that
// creates no child, publishes no suspension, and never parks on a person.
type RunProtocolProfile struct {
	ChildRuns      bool
	InterruptKinds []InterruptKind
}

// Clone returns an ownership-isolated copy of the protocol contract.
func (p RunProtocolProfile) Clone() RunProtocolProfile {
	p.InterruptKinds = slices.Clone(p.InterruptKinds)
	return p
}

// Validate reports whether the profile is in its one durable representation.
// Profiles cross process and persistence boundaries, so accepting multiple
// spellings of the same set would make equality depend on where a value was
// decoded. Admission normalizes once; every later boundary can then compare the
// frozen contract directly and fail closed on corruption.
func (p RunProtocolProfile) Validate() error {
	for index, kind := range p.InterruptKinds {
		if !kind.Valid() {
			return fmt.Errorf("execution: protocol profile interrupt kind[%d] is unknown", index)
		}
		if index > 0 && p.InterruptKinds[index-1] >= kind {
			return errors.New("execution: protocol profile interrupt kinds must be sorted without duplicates")
		}
	}
	return nil
}

// Equal reports semantic equality of two protocol contracts. It deliberately
// ignores order and duplicate spelling so callers validating an external value
// can explain a contract mismatch instead of accidentally comparing slice
// headers. Persisted/admitted profiles should additionally pass [Validate].
func (p RunProtocolProfile) Equal(other RunProtocolProfile) bool {
	left := p.Normalized()
	right := other.Normalized()
	return left.ChildRuns == right.ChildRuns && slices.Equal(left.InterruptKinds, right.InterruptKinds)
}

// Normalized returns the interrupt policy as a canonical set: sorted and without
// duplicates. Two profiles that differ only in interrupt order or repetition are
// the same contract, and comparing or storing them must not depend on how a caller
// happened to spell it.
func (p RunProtocolProfile) Normalized() RunProtocolProfile {
	kinds := slices.Clone(p.InterruptKinds)
	slices.Sort(kinds)
	return RunProtocolProfile{
		ChildRuns:      p.ChildRuns,
		InterruptKinds: slices.Compact(kinds),
	}
}

// IsEmpty reports the Minimal Profile — nothing negotiated, nothing to park on.
func (p RunProtocolProfile) IsEmpty() bool {
	return !p.ChildRuns && len(p.InterruptKinds) == 0
}

// Uncovered returns the parts of p that caller did not declare — the gap that
// makes a resume or a subscription a refusal rather than a downgraded stream
// (§8.1). It is profile-shaped because the gap IS a profile: the capabilities the
// caller would need before it could follow this Run.
func (p RunProtocolProfile) Uncovered(caller RunProtocolProfile) RunProtocolProfile {
	var gap RunProtocolProfile
	if p.ChildRuns && !caller.ChildRuns {
		gap.ChildRuns = true
	}
	for _, kind := range p.InterruptKinds {
		if !slices.Contains(caller.InterruptKinds, kind) {
			gap.InterruptKinds = append(gap.InterruptKinds, kind)
		}
	}
	return gap
}

// String names the profile's contents for an error explaining a refusal.
func (p RunProtocolProfile) String() string {
	parts := make([]string, 0, 1+len(p.InterruptKinds))
	if p.ChildRuns {
		parts = append(parts, "child runs")
	}
	for _, kind := range p.InterruptKinds {
		parts = append(parts, "interruptTypes."+kind.String())
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}
