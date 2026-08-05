package agent2

import "errors"

var ErrLimitExceeded = errors.New("agent: process limit exceeded")

// Limits bounds Framework-owned execution growth. Zero-valued fields in
// EngineConfig inherit DefaultLimits; Snapshot stores effective non-zero values
// so restoration preserves the same execution contract.
type Limits struct {
	// MaxSteps bounds committed Step transactions.
	MaxSteps uint64 `json:"max_steps"`

	// MaxEffects bounds Effects prepared across all Steps.
	MaxEffects uint64 `json:"max_effects"`

	// MaxSignals bounds all accepted external and Engine-generated Signals.
	MaxSignals uint64 `json:"max_signals"`

	// MaxPendingSignals bounds the unconsumed mailbox suffix, including space
	// reserved for the current prepared Effect batch.
	MaxPendingSignals uint64 `json:"max_pending_signals"`
}

// DefaultLimits returns conservative hard bounds for one Process.
func DefaultLimits() Limits {
	return Limits{
		MaxSteps: 10_000, MaxEffects: 10_000,
		MaxSignals: 100_000, MaxPendingSignals: 10_000,
	}
}

// Valid reports whether every growth dimension has a positive bound.
func (limits Limits) Valid() bool {
	return limits.MaxSteps > 0 && limits.MaxEffects > 0 &&
		limits.MaxSignals > 0 && limits.MaxPendingSignals > 0 &&
		limits.MaxPendingSignals <= limits.MaxSignals
}

func effectiveLimits(configured Limits) (Limits, error) {
	effective := configured
	defaults := DefaultLimits()
	if effective.MaxSteps == 0 {
		effective.MaxSteps = defaults.MaxSteps
	}
	if effective.MaxEffects == 0 {
		effective.MaxEffects = defaults.MaxEffects
	}
	if effective.MaxSignals == 0 {
		effective.MaxSignals = defaults.MaxSignals
	}
	if effective.MaxPendingSignals == 0 {
		effective.MaxPendingSignals = defaults.MaxPendingSignals
	}
	if !effective.Valid() {
		return Limits{}, ErrInvalidEngineConfig
	}
	return effective, nil
}

// Usage contains monotonic Framework-owned counters. It deliberately excludes
// provider pricing and Strategy-specific concepts such as tokens or tool calls.
type Usage struct {
	// CommittedSteps counts finalized Step transactions.
	CommittedSteps uint64 `json:"committed_steps"`

	// PreparedEffects counts stable logical Effect identities, not replay attempts.
	PreparedEffects uint64 `json:"prepared_effects"`

	// AcceptedSignals counts external and Engine-generated mailbox entries.
	AcceptedSignals uint64 `json:"accepted_signals"`

	// DroppedDeltas counts increments rejected by validation or the bounded queue.
	DroppedDeltas uint64 `json:"dropped_deltas"`
}

func (usage Usage) validFor(limits Limits) bool {
	return usage.CommittedSteps <= limits.MaxSteps &&
		usage.PreparedEffects <= limits.MaxEffects &&
		usage.AcceptedSignals <= limits.MaxSignals
}
