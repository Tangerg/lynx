package agent2

import (
	"errors"
	"fmt"
)

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

// Budget is a non-renewable allocation of Framework-owned work units. A child
// allocation is permanently transferred from its parent's remaining budget;
// unused units are not silently reclaimed or duplicated.
type Budget struct {
	Steps   uint64 `json:"steps"`
	Effects uint64 `json:"effects"`
	Signals uint64 `json:"signals"`
}

// NewBudget constructs one positive allocation.
func NewBudget(steps, effects, signals uint64) (Budget, error) {
	budget := Budget{Steps: steps, Effects: effects, Signals: signals}
	if !budget.Valid() {
		return Budget{}, ErrLimitExceeded
	}
	return budget, nil
}

// Valid reports whether every governed resource has a positive allocation.
func (budget Budget) Valid() bool {
	return budget.Steps > 0 && budget.Effects > 0 && budget.Signals > 0
}

func budgetFromLimits(limits Limits) Budget {
	return Budget{Steps: limits.MaxSteps, Effects: limits.MaxEffects, Signals: limits.MaxSignals}
}

func limitsFromBudget(parent Limits, budget Budget) (Limits, error) {
	if !budget.Valid() {
		return Limits{}, ErrLimitExceeded
	}
	pending := min(parent.MaxPendingSignals, budget.Signals)
	limits := Limits{
		MaxSteps: budget.Steps, MaxEffects: budget.Effects,
		MaxSignals: budget.Signals, MaxPendingSignals: pending,
	}
	if !limits.Valid() {
		return Limits{}, fmt.Errorf("%w: child budget cannot form valid limits", ErrLimitExceeded)
	}
	return limits, nil
}

func (budget Budget) contains(usage Usage, reserved Budget) bool {
	return usage.CommittedSteps <= budget.Steps && reserved.Steps <= budget.Steps-usage.CommittedSteps &&
		usage.PreparedEffects <= budget.Effects && reserved.Effects <= budget.Effects-usage.PreparedEffects &&
		usage.AcceptedSignals <= budget.Signals && reserved.Signals <= budget.Signals-usage.AcceptedSignals
}

func (budget Budget) canAllocate(usage Usage, reserved, requested Budget) bool {
	if !requested.Valid() || !budget.contains(usage, reserved) {
		return false
	}
	return requested.Steps <= budget.Steps-usage.CommittedSteps-reserved.Steps &&
		requested.Effects <= budget.Effects-usage.PreparedEffects-reserved.Effects &&
		requested.Signals <= budget.Signals-usage.AcceptedSignals-reserved.Signals
}

func (budget Budget) add(other Budget) (Budget, bool) {
	result := Budget{
		Steps: budget.Steps + other.Steps, Effects: budget.Effects + other.Effects,
		Signals: budget.Signals + other.Signals,
	}
	if result.Steps < budget.Steps || result.Effects < budget.Effects || result.Signals < budget.Signals {
		return Budget{}, false
	}
	return result, true
}

// TreeLimits bounds structural expansion independently of per-Process work
// limits. Every zero field in EngineConfig inherits DefaultTreeLimits.
type TreeLimits struct {
	MaxDepth          uint32 `json:"max_depth"`
	MaxChildren       uint32 `json:"max_children"`
	MaxActiveChildren uint32 `json:"max_active_children"`
	MaxTreeProcesses  uint32 `json:"max_tree_processes"`
}

// DefaultTreeLimits returns conservative structured-concurrency bounds.
func DefaultTreeLimits() TreeLimits {
	return TreeLimits{
		MaxDepth: 16, MaxChildren: 64, MaxActiveChildren: 16, MaxTreeProcesses: 1024,
	}
}

// Valid reports whether all tree growth dimensions are positive and coherent.
func (limits TreeLimits) Valid() bool {
	return limits.MaxDepth > 0 && limits.MaxChildren > 0 &&
		limits.MaxActiveChildren > 0 && limits.MaxActiveChildren <= limits.MaxChildren &&
		limits.MaxTreeProcesses > 0
}

func effectiveTreeLimits(configured TreeLimits) (TreeLimits, error) {
	effective := configured
	defaults := DefaultTreeLimits()
	if effective.MaxDepth == 0 {
		effective.MaxDepth = defaults.MaxDepth
	}
	if effective.MaxChildren == 0 {
		effective.MaxChildren = defaults.MaxChildren
	}
	if effective.MaxActiveChildren == 0 {
		effective.MaxActiveChildren = defaults.MaxActiveChildren
	}
	if effective.MaxTreeProcesses == 0 {
		effective.MaxTreeProcesses = defaults.MaxTreeProcesses
	}
	if !effective.Valid() {
		return TreeLimits{}, ErrInvalidEngineConfig
	}
	return effective, nil
}
