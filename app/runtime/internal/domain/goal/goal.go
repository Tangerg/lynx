// Package goal is the autonomous-execution loop's durable state: at most one
// goal per session that drives runs toward an objective until the model signals
// it complete or blocked, an opt-in budget is spent, or the user stops it. The
// GoalDriver (application/goals) owns the loop; this package holds the entity,
// its status vocabulary, and the cross-turn budget accounting. A goal is
// deliberately session-scoped, not run-scoped: it spans the
// many runs the loop launches, so it lives outside the per-run execution.RunState
// machine (which has no paused state and terminalizes a lost run on restart).
package goal

import (
	"errors"
	"fmt"
	"time"
)

// Status is where a goal sits in the autonomous loop.
//
// StatusComplete is transient: the model sets it through the update_goal tool,
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

// Budget is the opt-in cross-turn cap. A zero field is unbounded on that axis;
// an all-zero Budget lets the loop run until the model declares done or the user
// stops it (the entry gate makes that an explicit choice).
type Budget struct {
	MaxTurns   int     // total autonomous turns
	MaxCostUSD float64 // summed USD across turns
	MaxSteps   int     // summed model calls across turns
}

// Usage accumulates what the loop has spent across its turns so far.
type Usage struct {
	Turns   int
	CostUSD float64
	Steps   int
}

// Version identifies one durable revision of a Goal. LeaseID is an opaque,
// non-reusable ownership token for a driving loop; Revision advances on every
// persisted mutation within that lease. Together they make a stale loop unable
// to write a freshly-created Goal after the old row was cleared.
type Version struct {
	LeaseID  string
	Revision int64
}

// BudgetLimit identifies the cross-turn cap that stopped a goal.
type BudgetLimit uint8

const (
	BudgetLimitNone BudgetLimit = iota
	BudgetLimitTurns
	BudgetLimitCost
	BudgetLimitSteps
)

// Exceeded reports the first budget limit u has reached, or (BudgetLimitNone,
// false) when the goal is still within budget. Checked after each turn commits
// its usage.
func (b Budget) Exceeded(u Usage) (limit BudgetLimit, exceeded bool) {
	switch {
	case b.MaxTurns > 0 && u.Turns >= b.MaxTurns:
		return BudgetLimitTurns, true
	case b.MaxCostUSD > 0 && u.CostUSD >= b.MaxCostUSD:
		return BudgetLimitCost, true
	case b.MaxSteps > 0 && u.Steps >= b.MaxSteps:
		return BudgetLimitSteps, true
	default:
		return BudgetLimitNone, false
	}
}

// ReasonCause classifies why a paused or blocked goal stopped. It deliberately
// carries no display text: delivery maps the stable cause to client wording.
type ReasonCause uint8

const (
	ReasonNone ReasonCause = iota
	ReasonStoppedByUser
	ReasonRuntimeRestarted
	ReasonRunStartFailed
	ReasonAwaitingInput
	ReasonTerminalOutcomeMissing
	ReasonRunNotCompleted
	ReasonTurnBudgetReached
	ReasonCostBudgetReached
	ReasonStepBudgetReached
	ReasonBlockedByModel
)

// Valid reports whether c is a recognized stopping cause.
func (c ReasonCause) Valid() bool {
	return c >= ReasonNone && c <= ReasonBlockedByModel
}

// Reason is the typed stopping context stored with a paused or blocked goal.
// Detail is allowed only for model-authored explanations and stable domain
// values such as an Outcome string. Infrastructure errors belong in logs and
// traces, never in durable goal state.
type Reason struct {
	Cause  ReasonCause
	Detail string
}

// Goal is one session's autonomous objective and loop state.
type Goal struct {
	SessionID string
	Objective string
	Status    Status
	Reason    Reason // why it is paused or blocked; zero while active
	Provider  string // model the loop runs each turn against
	Model     string
	Budget    Budget
	Used      Usage
	// LeaseID names the currently valid driving-loop incarnation. It is generated
	// afresh at every lifecycle transition, never inferred from row existence.
	LeaseID string
	// Revision is the durable optimistic-concurrency version for mutations made
	// within a single lease.
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	errSessionRequired   = errors.New("goal: session ID is required")
	errObjectiveRequired = errors.New("goal: objective is required")
	errInvalidSnapshot   = errors.New("goal: invalid snapshot")
)

// New builds an active goal for sessionID. now is passed in (not read from the
// clock) so callers stay testable and the runtime keeps a single time source.
func New(sessionID, objective, provider, model string, budget Budget, now time.Time) (Goal, error) {
	if sessionID == "" {
		return Goal{}, errSessionRequired
	}
	if objective == "" {
		return Goal{}, errObjectiveRequired
	}
	return Goal{
		SessionID: sessionID,
		Objective: objective,
		Status:    StatusActive,
		Provider:  provider,
		Model:     model,
		Budget:    budget,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ValidateSnapshot verifies the invariants of one durable goal state. It does
// not validate a lifecycle transition; persistence adapters use it when they
// reconstruct a Goal so corrupt or obsolete rows cannot enter the application.
func (g Goal) ValidateSnapshot() error {
	if g.SessionID == "" {
		return errSessionRequired
	}
	if g.Objective == "" {
		return errObjectiveRequired
	}
	if !g.Status.Valid() {
		return fmt.Errorf("%w: unknown status %q", errInvalidSnapshot, g.Status)
	}
	if !g.Reason.Cause.Valid() {
		return fmt.Errorf("%w: unknown reason cause %d", errInvalidSnapshot, g.Reason.Cause)
	}
	if g.Budget.MaxTurns < 0 || g.Budget.MaxCostUSD < 0 || g.Budget.MaxSteps < 0 {
		return fmt.Errorf("%w: negative budget", errInvalidSnapshot)
	}
	if g.Used.Turns < 0 || g.Used.CostUSD < 0 || g.Used.Steps < 0 {
		return fmt.Errorf("%w: negative usage", errInvalidSnapshot)
	}
	switch g.Status {
	case StatusActive, StatusComplete:
		if g.Reason.Cause != ReasonNone || g.Reason.Detail != "" {
			return fmt.Errorf("%w: %s goal must not carry a stop reason", errInvalidSnapshot, g.Status)
		}
	case StatusPaused, StatusBlocked:
		if g.Reason.Cause == ReasonNone {
			return fmt.Errorf("%w: %s goal requires a stop reason", errInvalidSnapshot, g.Status)
		}
	}
	return nil
}

// AddTurn folds one completed turn's usage into the accumulator.
func (g *Goal) AddTurn(costUSD float64, steps int, now time.Time) {
	g.Used.Turns++
	g.Used.CostUSD += costUSD
	g.Used.Steps += steps
	g.UpdatedAt = now
}

// Complete marks the objective done. It is a transient state: the driver
// observes it once, announces, and clears the goal — a completed goal is never a
// durable resting state (see [Status]).
func (g *Goal) Complete(now time.Time) {
	g.Status = StatusComplete
	g.Reason = Reason{}
	g.UpdatedAt = now
}

// Pause stops the loop with a typed cause (user stop, restart degrade, a run
// that parked for HITL, or a transient run error). A paused goal can be resumed.
func (g *Goal) Pause(cause ReasonCause, detail string, now time.Time) {
	g.Status = StatusPaused
	g.Reason = Reason{Cause: cause, Detail: detail}
	g.UpdatedAt = now
}

// Block records a typed deadlock the user must resolve (budget spent, or the
// model declared itself stuck). Like paused, it is resumable, but it signals
// the loop stopped itself rather than the user stopping it.
func (g *Goal) Block(cause ReasonCause, detail string, now time.Time) {
	g.Status = StatusBlocked
	g.Reason = Reason{Cause: cause, Detail: detail}
	g.UpdatedAt = now
}

// Resume returns a paused or blocked goal to active so the driver drives it again.
func (g *Goal) Resume(now time.Time) {
	g.Status = StatusActive
	g.Reason = Reason{}
	g.UpdatedAt = now
}

// Version returns the value a caller must use to condition its next mutation.
func (g Goal) Version() Version {
	return Version{LeaseID: g.LeaseID, Revision: g.Revision}
}

// RenewLease revokes every prior loop ownership token and advances the durable
// revision. Lifecycle transitions call it before persisting their new state.
func (g *Goal) RenewLease(leaseID string) {
	g.LeaseID = leaseID
	g.Revision++
}

// AdvanceRevision records a mutation within the current lease.
func (g *Goal) AdvanceRevision() { g.Revision++ }
