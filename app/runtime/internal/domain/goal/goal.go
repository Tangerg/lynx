// Package goal is the autonomous-execution loop's durable state: at most one
// goal per session that drives runs toward an objective until the model signals
// it complete or blocked, an opt-in budget is spent, or the user stops it.
// The use case driving Runs owns the loop; this package holds the entity,
// its status vocabulary, and the cross-Run budget accounting. A goal is
// deliberately session-scoped, not run-scoped: it spans the
// many runs the loop launches, so it lives outside the per-run run.State
// machine (which has no paused state and terminalizes a lost run on restart).
package goal

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
)

// Status is where a goal sits in the autonomous loop.
//
// StatusComplete is transient: the model reports it through the terminal-outcome boundary,
// the driver observes it once and clears the goal. It is never a durable
// resting state — a stored complete goal only exists in the window between the
// tool call and the driver's next read (or a crash in that window, which the
// boot reconcile clears).
type Status string

const (
	StatusActive   Status = "active"   // the loop is (or should be) driving runs
	StatusPaused   Status = "paused"   // the user stopped it, or a restart degraded it
	StatusBlocked  Status = "blocked"  // a deadlock the user must resolve (budget / model-declared)
	StatusComplete Status = "complete" // transient: announced, then cleared
)

// Valid reports whether s is a recognized status.
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusBlocked, StatusComplete:
		return true
	default:
		return false
	}
}

// Budget is the opt-in cross-Run cap. A zero field is unbounded on that axis;
// an all-zero Budget lets the loop run until the model declares done or the user
// stops it (the entry gate makes that an explicit choice).
type Budget struct {
	MaxRuns    int     // total autonomous Runs
	MaxCostUSD float64 // summed USD across Runs
	MaxSteps   int     // summed model calls across Runs
}

// Validate reports whether every configured budget ceiling is finite and
// non-negative.
func (b Budget) Validate() error {
	if b.MaxRuns < 0 || b.MaxSteps < 0 || b.MaxCostUSD < 0 ||
		math.IsNaN(b.MaxCostUSD) || math.IsInf(b.MaxCostUSD, 0) {
		return errors.New("goal: budget limits must be finite and non-negative")
	}
	return nil
}

// Usage accumulates what the loop has spent across its Runs so far.
type Usage struct {
	Runs    int
	CostUSD float64
	Steps   int
}

// Version identifies one durable revision of a Goal. IncarnationID names the
// immutable objective incarnation; Revision advances on every persisted
// mutation inside it. Together they prevent an old Run or drive from writing a
// fresh Goal that replaced the prior objective after its row was cleared.
type Version struct {
	IncarnationID string
	Revision      int64
}

// BudgetLimit identifies the cross-Run cap that stopped a goal.
type BudgetLimit string

const (
	BudgetLimitNone  BudgetLimit = ""
	BudgetLimitRuns  BudgetLimit = "runs"
	BudgetLimitCost  BudgetLimit = "cost"
	BudgetLimitSteps BudgetLimit = "steps"
)

// Valid reports whether b identifies a supported budget axis or no limit.
func (b BudgetLimit) Valid() bool {
	return b == BudgetLimitNone || b == BudgetLimitRuns || b == BudgetLimitCost || b == BudgetLimitSteps
}

// Exceeded reports the first budget limit u has reached, or (BudgetLimitNone,
// false) when the goal is still within budget. Checked after each Run commits
// its usage.
func (b Budget) Exceeded(u Usage) (limit BudgetLimit, exceeded bool) {
	switch {
	case b.MaxRuns > 0 && u.Runs >= b.MaxRuns:
		return BudgetLimitRuns, true
	case b.MaxCostUSD > 0 && u.CostUSD >= b.MaxCostUSD:
		return BudgetLimitCost, true
	case b.MaxSteps > 0 && u.Steps >= b.MaxSteps:
		return BudgetLimitSteps, true
	default:
		return BudgetLimitNone, false
	}
}

// ReasonCode classifies why a paused or blocked goal stopped. Its stable string
// value is safe to persist and project across process boundaries; presentation
// layers decide how to explain it to their audience.
type ReasonCode string

const (
	ReasonNone                   ReasonCode = ""
	ReasonStoppedByUser          ReasonCode = "stoppedByUser"
	ReasonRuntimeRestarted       ReasonCode = "runtimeRestarted"
	ReasonRunStartFailed         ReasonCode = "runStartFailed"
	ReasonAwaitingInput          ReasonCode = "awaitingInput"
	ReasonTerminalOutcomeMissing ReasonCode = "terminalOutcomeMissing"
	ReasonRunNotCompleted        ReasonCode = "runNotCompleted"
	ReasonRunBudgetReached       ReasonCode = "runBudgetReached"
	ReasonCostBudgetReached      ReasonCode = "costBudgetReached"
	ReasonStepBudgetReached      ReasonCode = "stepBudgetReached"
	ReasonBlockedByModel         ReasonCode = "blockedByModel"
)

// Valid reports whether r is a recognized stopping reason.
func (r ReasonCode) Valid() bool {
	switch r {
	case ReasonNone,
		ReasonStoppedByUser,
		ReasonRuntimeRestarted,
		ReasonRunStartFailed,
		ReasonAwaitingInput,
		ReasonTerminalOutcomeMissing,
		ReasonRunNotCompleted,
		ReasonRunBudgetReached,
		ReasonCostBudgetReached,
		ReasonStepBudgetReached,
		ReasonBlockedByModel:
		return true
	default:
		return false
	}
}

// Reason is the typed stopping context stored with a paused or blocked goal.
// Detail is allowed only for model-authored explanations and stable domain
// values such as an Outcome string. Operational errors belong in logs and
// traces, never in durable goal state.
type Reason struct {
	Code   ReasonCode
	Detail string
}

// Goal is one session's autonomous objective and loop state.
type Goal struct {
	SessionID      string
	Objective      string
	Status         Status
	Reason         Reason             // why it is paused or blocked; zero while active
	ModelSelection modelref.Selection // model the loop runs each Run against
	// Capabilities is the client contract frozen when this objective incarnation
	// starts. Every autonomous Run uses the same set; a later resume may prove it
	// can cover the set but cannot renegotiate it.
	Capabilities run.Capabilities
	Budget       Budget
	Used         Usage
	// IncarnationID names this objective incarnation. Pausing and resuming do not
	// replace the objective, so every Run already admitted for it keeps the same
	// identity until a fresh Goal replaces the aggregate.
	IncarnationID string
	// Revision is the durable optimistic-concurrency version of this session's
	// goal row. Persistence, not callers, assigns and advances it.
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	errSessionRequired   = errors.New("goal: session ID is required")
	errObjectiveRequired = errors.New("goal: objective is required")
	errInvalidSnapshot   = errors.New("goal: invalid snapshot")
	// ErrBudgetExhausted rejects a resume that would start work beyond the
	// durable cross-Run budget. Changing a budget is a separate intent, never
	// an implicit side effect of resuming a blocked goal.
	ErrBudgetExhausted = errors.New("goal: budget exhausted")
	// ErrNotResumable rejects a lifecycle transition from a terminal/transient
	// status. A complete goal is cleared rather than revived.
	ErrNotResumable = errors.New("goal: status is not resumable")
	// ErrNotEditable rejects an objective edit during the transient completion
	// settlement window. The owning drive must finish accounting and clear it.
	ErrNotEditable = errors.New("goal: status is not editable")
	// ErrRunIdentityConflict reports an attempt to reuse one Run identity for a
	// different immutable Goal accounting fact. Exact retries are idempotent;
	// conflicting retries are corruption and must never be silently accepted.
	ErrRunIdentityConflict = errors.New("goal: Run identity conflict")
)

// New builds a fresh active objective incarnation for sessionID. The
// incarnation is part of the aggregate rather than a follow-up mutation, so an
// admitted Run can always carry exact Goal provenance. Persistence assigns the
// first durable revision.
func New(
	sessionID, objective string,
	selection modelref.Selection,
	budget Budget,
	capabilities run.Capabilities,
	incarnationID string,
	now time.Time,
) (Goal, error) {
	if sessionID == "" {
		return Goal{}, errSessionRequired
	}
	if objective == "" {
		return Goal{}, errObjectiveRequired
	}
	if incarnationID == "" {
		return Goal{}, fmt.Errorf("%w: incarnation ID is required", errInvalidSnapshot)
	}
	if err := budget.Validate(); err != nil {
		return Goal{}, err
	}
	capabilities = capabilities.Normalized()
	if err := capabilities.Validate(); err != nil {
		return Goal{}, fmt.Errorf("goal: capabilities: %w", err)
	}
	return Goal{
		SessionID:      sessionID,
		Objective:      objective,
		Status:         StatusActive,
		ModelSelection: selection,
		Capabilities:   capabilities,
		Budget:         budget,
		IncarnationID:  incarnationID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// ValidateSnapshot verifies the invariants of one durable goal state. It does
// not validate a lifecycle transition; persistence reconstruction uses it so
// corrupt or obsolete rows cannot become a Goal.
func (g Goal) ValidateSnapshot() error {
	if g.SessionID == "" {
		return errSessionRequired
	}
	if g.Objective == "" {
		return errObjectiveRequired
	}
	if g.IncarnationID == "" {
		return fmt.Errorf("%w: incarnation ID is required", errInvalidSnapshot)
	}
	if g.Revision <= 0 {
		return fmt.Errorf("%w: revision must be positive", errInvalidSnapshot)
	}
	if !g.Status.Valid() {
		return fmt.Errorf("%w: unknown status %q", errInvalidSnapshot, g.Status)
	}
	if !g.Reason.Code.Valid() {
		return fmt.Errorf("%w: unknown reason code %q", errInvalidSnapshot, g.Reason.Code)
	}
	if err := g.Budget.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errInvalidSnapshot, err)
	}
	if err := g.Capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: capabilities: %w", errInvalidSnapshot, err)
	}
	if g.Used.Runs < 0 || g.Used.CostUSD < 0 || g.Used.Steps < 0 ||
		math.IsNaN(g.Used.CostUSD) || math.IsInf(g.Used.CostUSD, 0) {
		return fmt.Errorf("%w: usage must be finite and non-negative", errInvalidSnapshot)
	}
	switch g.Status {
	case StatusActive, StatusComplete:
		if g.Reason.Code != ReasonNone || g.Reason.Detail != "" {
			return fmt.Errorf("%w: %s goal must not carry a stop reason", errInvalidSnapshot, g.Status)
		}
	case StatusPaused, StatusBlocked:
		if g.Reason.Code == ReasonNone {
			return fmt.Errorf("%w: %s goal requires a stop reason", errInvalidSnapshot, g.Status)
		}
	}
	return nil
}

// Clone returns an ownership-isolated Goal value.
func (g Goal) Clone() Goal {
	g.Capabilities = g.Capabilities.Clone()
	return g
}

// AddRun folds one completed Run's usage into the accumulator.
func (g *Goal) AddRun(costUSD float64, steps int, now time.Time) {
	g.Used.Runs++
	g.Used.CostUSD += costUSD
	g.Used.Steps += steps
	g.UpdatedAt = now
}

// RunRecord is the immutable accounting fact emitted when one goal-owned Run
// terminalizes. RunID makes the store-level recording idempotent;
// IncarnationID keeps a Run from charging a later objective incarnation.
type RunRecord struct {
	SessionID     string
	IncarnationID string
	RunID         string
	Outcome       run.Outcome
	CostUSD       float64
	Steps         int
	CompletedAt   time.Time
}

// Validate reports whether this immutable Goal accounting fact is complete and
// numerically representable before it reaches the idempotency ledger.
func (r RunRecord) Validate() error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session ID", value: r.SessionID},
		{name: "incarnation ID", value: r.IncarnationID},
		{name: "Run ID", value: r.RunID},
	} {
		if strings.TrimSpace(identity.value) == "" {
			return fmt.Errorf("goal: Run %s is required", identity.name)
		}
		if identity.value != strings.TrimSpace(identity.value) {
			return fmt.Errorf("goal: Run %s has surrounding whitespace", identity.name)
		}
	}
	if _, ok := run.ParseOutcome(r.Outcome.String()); !ok {
		return fmt.Errorf("goal: Run has unknown outcome %q", r.Outcome)
	}
	if r.CostUSD < 0 || math.IsNaN(r.CostUSD) || math.IsInf(r.CostUSD, 0) {
		return errors.New("goal: Run cost must be a finite non-negative number")
	}
	if r.Steps < 0 {
		return errors.New("goal: Run steps must not be negative")
	}
	if r.CompletedAt.IsZero() {
		return errors.New("goal: Run completion time is required")
	}
	return nil
}

// RecordRun folds one terminal Run into the matching Goal incarnation. It
// always records work that incarnation performed; a model report may already have
// changed Active to Complete or Blocked before the Run terminalizes, and that
// transition must not erase the Run's cost. Only an active goal derives a new
// lifecycle state from the terminal outcome or budget — an earlier explicit
// stop/report remains authoritative.
//
// Callers persist this mutation in the same transaction that terminalizes the
// Run, so a completed Run and its budget charge cannot diverge.
func (g *Goal) RecordRun(record RunRecord) {
	g.AddRun(record.CostUSD, record.Steps, record.CompletedAt)
	if g.Status != StatusActive {
		return
	}
	if record.Outcome != run.OutcomeCompleted {
		g.Pause(ReasonRunNotCompleted, record.Outcome.String(), record.CompletedAt)
		return
	}
	if limit, over := g.Budget.Exceeded(g.Used); over {
		g.Block(reasonForBudgetLimit(limit), "", record.CompletedAt)
	}
}

// Complete marks the objective done. It is a transient state: the driver
// observes it once, announces, and clears the goal — a completed goal is never a
// durable resting state (see [Status]).
func (g *Goal) Complete(now time.Time) {
	g.Status = StatusComplete
	g.Reason = Reason{}
	g.UpdatedAt = now
}

// Pause stops the loop with a typed reason (user stop, restart degrade, a run
// that parked for HITL, or a transient run error). A paused goal can be resumed.
func (g *Goal) Pause(code ReasonCode, detail string, now time.Time) {
	g.Status = StatusPaused
	g.Reason = Reason{Code: code, Detail: detail}
	g.UpdatedAt = now
}

// Block records a typed deadlock the user must resolve (budget spent, or the
// model declared itself stuck). Model-declared blocks may be resumed; a spent
// budget is deliberately not resumable because another Run would exceed the
// configured cap before it could be accounted.
func (g *Goal) Block(code ReasonCode, detail string, now time.Time) {
	g.Status = StatusBlocked
	g.Reason = Reason{Code: code, Detail: detail}
	g.UpdatedAt = now
}

// Resume returns a paused or blocked goal to active so the driver drives it
// again. A spent budget is not resumable: starting another Run would violate
// the aggregate's durable cap before any later accounting can stop it.
func (g *Goal) Resume(now time.Time) error {
	if g.Status != StatusPaused && g.Status != StatusBlocked {
		return ErrNotResumable
	}
	if _, exhausted := g.Budget.Exceeded(g.Used); exhausted {
		return ErrBudgetExhausted
	}
	g.Status = StatusActive
	g.Reason = Reason{}
	g.UpdatedAt = now
	return nil
}

// ReviseObjective replaces only the user-authored objective while preserving
// the Goal's lifecycle, frozen execution settings, accounting and creation
// time. A fresh incarnation severs any stale Run provenance from the prior
// objective; persistence assigns the next durable revision.
func (g *Goal) ReviseObjective(objective, incarnationID string, now time.Time) error {
	if objective == "" {
		return errObjectiveRequired
	}
	if incarnationID == "" {
		return fmt.Errorf("%w: incarnation ID is required", errInvalidSnapshot)
	}
	if g.Status == StatusComplete {
		return ErrNotEditable
	}
	g.Objective = objective
	g.IncarnationID = incarnationID
	g.UpdatedAt = now
	return nil
}

func reasonForBudgetLimit(limit BudgetLimit) ReasonCode {
	switch limit {
	case BudgetLimitRuns:
		return ReasonRunBudgetReached
	case BudgetLimitCost:
		return ReasonCostBudgetReached
	case BudgetLimitSteps:
		return ReasonStepBudgetReached
	default:
		return ReasonNone
	}
}

// Version returns the value a caller must use to condition its next mutation.
func (g Goal) Version() Version {
	return Version{IncarnationID: g.IncarnationID, Revision: g.Revision}
}
