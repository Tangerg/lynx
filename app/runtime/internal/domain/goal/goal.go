// Package goal is the autonomous-execution loop's durable state: at most one
// goal per session that drives runs toward an objective until the model signals
// it complete or blocked, an opt-in budget is spent, or the user stops it.
// The use case driving Runs owns the loop; this package holds the entity,
// its status vocabulary, and the cross-Run budget accounting. A goal is
// deliberately session-scoped, not run-scoped: it spans the
// many runs the loop launches, so it lives outside the per-run execution.RunState
// machine (which has no paused state and terminalizes a lost run on restart).
package goal

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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

// Version identifies one durable revision of a Goal. LeaseID is an opaque,
// non-reusable ownership token for a driving loop; Revision advances on every
// persisted mutation, including lifecycle transitions that renew the lease.
// Together they make a stale loop unable to write a freshly-created Goal after
// the old row was cleared.
type Version struct {
	LeaseID  string
	Revision int64
}

// BudgetLimit identifies the cross-Run cap that stopped a goal.
type BudgetLimit uint8

const (
	BudgetLimitNone BudgetLimit = iota
	BudgetLimitRuns
	BudgetLimitCost
	BudgetLimitSteps
)

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

// Valid reports whether code is a recognized stopping reason.
func (code ReasonCode) Valid() bool {
	switch code {
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
	Budget         Budget
	Used           Usage
	// LeaseID names the currently valid driving-loop incarnation. It is generated
	// afresh at every lifecycle transition, never inferred from row existence.
	LeaseID string
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
	// ErrRunIdentityConflict reports an attempt to reuse one Run identity for a
	// different immutable Goal accounting fact. Exact retries are idempotent;
	// conflicting retries are corruption and must never be silently accepted.
	ErrRunIdentityConflict = errors.New("goal: Run identity conflict")
)

// New builds a new active goal for sessionID. A lease is part of the aggregate
// rather than a follow-up mutation: an active goal without an owner is not a
// valid intermediate state. Persistence assigns the first durable revision.
func New(sessionID, objective string, selection modelref.Selection, budget Budget, leaseID string, now time.Time) (Goal, error) {
	if sessionID == "" {
		return Goal{}, errSessionRequired
	}
	if objective == "" {
		return Goal{}, errObjectiveRequired
	}
	if leaseID == "" {
		return Goal{}, fmt.Errorf("%w: lease ID is required", errInvalidSnapshot)
	}
	if err := budget.Validate(); err != nil {
		return Goal{}, err
	}
	return Goal{
		SessionID:      sessionID,
		Objective:      objective,
		Status:         StatusActive,
		ModelSelection: selection,
		Budget:         budget,
		LeaseID:        leaseID,
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
	if g.LeaseID == "" {
		return fmt.Errorf("%w: lease ID is required", errInvalidSnapshot)
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

// AddRun folds one completed Run's usage into the accumulator.
func (g *Goal) AddRun(costUSD float64, steps int, now time.Time) {
	g.Used.Runs++
	g.Used.CostUSD += costUSD
	g.Used.Steps += steps
	g.UpdatedAt = now
}

// RunRecord is the immutable accounting fact emitted when one goal-owned Run
// terminalizes. RunID makes the store-level recording idempotent; LeaseID keeps
// a straggling Run from charging a later goal incarnation.
type RunRecord struct {
	SessionID   string
	LeaseID     string
	RunID       string
	Outcome     execution.Outcome
	CostUSD     float64
	Steps       int
	CompletedAt time.Time
}

// Validate reports whether this immutable Goal accounting fact is complete and
// numerically representable before it reaches the idempotency ledger.
func (record RunRecord) Validate() error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session ID", value: record.SessionID},
		{name: "lease ID", value: record.LeaseID},
		{name: "Run ID", value: record.RunID},
	} {
		if strings.TrimSpace(identity.value) == "" {
			return fmt.Errorf("goal: Run %s is required", identity.name)
		}
		if identity.value != strings.TrimSpace(identity.value) {
			return fmt.Errorf("goal: Run %s has surrounding whitespace", identity.name)
		}
	}
	if _, ok := execution.ParseOutcome(record.Outcome.String()); !ok {
		return fmt.Errorf("goal: Run has unknown outcome %d", record.Outcome)
	}
	if record.CostUSD < 0 || math.IsNaN(record.CostUSD) || math.IsInf(record.CostUSD, 0) {
		return errors.New("goal: Run cost must be a finite non-negative number")
	}
	if record.Steps < 0 {
		return errors.New("goal: Run steps must not be negative")
	}
	if record.CompletedAt.IsZero() {
		return errors.New("goal: Run completion time is required")
	}
	return nil
}

// RecordRun folds one terminal Run into the matching goal lease. It always
// records work the lease actually performed; a model report may already have
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
	if record.Outcome != execution.OutcomeCompleted {
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
	return Version{LeaseID: g.LeaseID, Revision: g.Revision}
}

// RenewLease revokes every prior loop ownership token. Persistence owns revision
// advancement, so lifecycle code cannot accidentally publish an unversioned
// mutation or advance a version twice.
func (g *Goal) RenewLease(leaseID string) {
	g.LeaseID = leaseID
}
