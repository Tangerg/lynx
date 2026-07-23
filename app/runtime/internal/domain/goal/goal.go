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

// Exceeded reports the first budget axis u has reached, or ("", false) when the
// goal is still within budget. Checked after each turn commits its usage.
func (b Budget) Exceeded(u Usage) (axis string, exceeded bool) {
	switch {
	case b.MaxTurns > 0 && u.Turns >= b.MaxTurns:
		return "turn", true
	case b.MaxCostUSD > 0 && u.CostUSD >= b.MaxCostUSD:
		return "cost", true
	case b.MaxSteps > 0 && u.Steps >= b.MaxSteps:
		return "step", true
	default:
		return "", false
	}
}

// Goal is one session's autonomous objective and loop state.
type Goal struct {
	SessionID string
	Objective string
	Status    Status
	Reason    string // why it is paused or blocked; empty while active
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
	g.Reason = ""
	g.UpdatedAt = now
}

// Pause stops the loop with a reason (user stop, restart degrade, a run that
// parked for HITL, or a transient run error). A paused goal can be resumed.
func (g *Goal) Pause(reason string, now time.Time) {
	g.Status = StatusPaused
	g.Reason = reason
	g.UpdatedAt = now
}

// Block records a deadlock the user must resolve (budget spent, or the model
// declared itself stuck). Like paused, it is resumable, but it signals the loop
// stopped itself rather than the user stopping it.
func (g *Goal) Block(reason string, now time.Time) {
	g.Status = StatusBlocked
	g.Reason = reason
	g.UpdatedAt = now
}

// Resume returns a paused or blocked goal to active so the driver drives it again.
func (g *Goal) Resume(now time.Time) {
	g.Status = StatusActive
	g.Reason = ""
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
