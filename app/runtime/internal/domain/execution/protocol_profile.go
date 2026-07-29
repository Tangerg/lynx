package execution

import (
	"errors"
	"slices"
	"strings"
)

// ErrProfileNotCovered reports that a caller cannot follow a Run: the Run's frozen
// profile names capabilities the caller did not declare. It lives with the profile
// rather than with either use case, because both the continuation path and the
// waiting-set read enforce the same rule, and one rule has one sentinel.
var ErrProfileNotCovered = errors.New("execution: caller does not cover the run's protocol profile")

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
// RequiredFeatures are protocol capability keys, kept as opaque strings on
// purpose: the vocabulary is the wire's and the domain does not interpret it. What
// the domain owns is that the set was frozen and is reported back unchanged, so no
// layer can quietly widen or narrow it. The empty profile is not "unset" — it is
// the Minimal Profile: a Run that creates no child, publishes no suspension, and
// never parks on a person.
type RunProtocolProfile struct {
	RequiredFeatures []string
	InterruptKinds   []InterruptKind
}

// Normalized returns the profile as a canonical set: sorted, without duplicates,
// and with empty slices rather than nil. Both fields are sets, so two profiles
// that differ only in order or repetition are the same contract, and comparing or
// storing them must not depend on how a caller happened to spell it.
func (p RunProtocolProfile) Normalized() RunProtocolProfile {
	features := slices.Clone(p.RequiredFeatures)
	slices.Sort(features)
	kinds := slices.Clone(p.InterruptKinds)
	slices.Sort(kinds)
	return RunProtocolProfile{
		RequiredFeatures: slices.Compact(features),
		InterruptKinds:   slices.Compact(kinds),
	}
}

// IsEmpty reports the Minimal Profile — nothing negotiated, nothing to park on.
func (p RunProtocolProfile) IsEmpty() bool {
	return len(p.RequiredFeatures) == 0 && len(p.InterruptKinds) == 0
}

// Uncovered returns the parts of p that caller did not declare — the gap that
// makes a resume or a subscription a refusal rather than a downgraded stream
// (§8.1). It is profile-shaped because the gap IS a profile: the capabilities the
// caller would need before it could follow this Run.
func (p RunProtocolProfile) Uncovered(caller RunProtocolProfile) RunProtocolProfile {
	var gap RunProtocolProfile
	for _, feature := range p.RequiredFeatures {
		if !slices.Contains(caller.RequiredFeatures, feature) {
			gap.RequiredFeatures = append(gap.RequiredFeatures, feature)
		}
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
	parts := make([]string, 0, len(p.RequiredFeatures)+len(p.InterruptKinds))
	for _, feature := range p.RequiredFeatures {
		parts = append(parts, "features."+feature)
	}
	for _, kind := range p.InterruptKinds {
		parts = append(parts, "interruptTypes."+kind.String())
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}
