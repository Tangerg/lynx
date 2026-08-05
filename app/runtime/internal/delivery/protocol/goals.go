package protocol

import (
	"context"
	"time"
)

// Goals is the goals.* method group — Goal mode, an autonomous execution loop
// that drives runs toward an objective until the model signals it complete or
// blocked (via report_goal_outcome), an opt-in cross-Run budget is spent, or
// the user stops it. A session has at most one goal. Starting a goal is the
// explicit opt-in gate; while it runs, the runtime launches runs back-to-back on
// its own instead of the user driving each Run.
type Goals interface {
	// StartGoal opens a goal for the session and begins driving it. sessionId +
	// objective are required; provider/model pair the model each Run uses
	// against; budget caps the loop (all-zero = uncapped, an explicit choice).
	// Fails if the session already has an actively-driving goal.
	StartGoal(ctx context.Context, in StartGoalRequest) (*Goal, error)
	// GetGoal returns the session's goal, or a nil result when it has none.
	GetGoal(ctx context.Context, in GoalRequest) (*Goal, error)
	// StopGoal cancels and joins the in-flight run, then pauses the goal without
	// losing terminal accounting that completed during cancellation.
	StopGoal(ctx context.Context, in GoalRequest) (*Goal, error)
	// ResumeGoal returns a paused or blocked goal to active and drives it again.
	ResumeGoal(ctx context.Context, in GoalRequest) (*Goal, error)
}

// Goal is one session's autonomous objective and loop state (API.md §7.14).
// status is active | paused | blocked; a completed goal is cleared, so it never
// appears here.
type Goal struct {
	SessionID string      `json:"sessionId"`
	Objective string      `json:"objective"`
	Status    GoalStatus  `json:"status"`
	Reason    *GoalReason `json:"reason,omitempty"`
	Provider  string      `json:"provider,omitempty"`
	Model     string      `json:"model,omitempty"`
	Budget    GoalBudget  `json:"budget"`
	Used      GoalUsage   `json:"used"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

// GoalStatus is the durable resting-state vocabulary exposed by the
// autonomous-goal API. The domain's complete state is intentionally absent:
// it is a loop-internal transient that is cleared before a client can observe
// a goal again.
type GoalStatus string

const (
	GoalActive  GoalStatus = "active"
	GoalPaused  GoalStatus = "paused"
	GoalBlocked GoalStatus = "blocked"
)

// GoalReason is machine-readable stopping context. Code determines client
// presentation and behavior; detail carries only safe domain/model context.
type GoalReason struct {
	Code   GoalReasonCode `json:"code"`
	Detail string         `json:"detail,omitempty"`
}

// GoalReasonCode is the closed vocabulary for paused and blocked goals.
type GoalReasonCode string

const (
	GoalReasonStoppedByUser          GoalReasonCode = "stoppedByUser"
	GoalReasonRuntimeRestarted       GoalReasonCode = "runtimeRestarted"
	GoalReasonRunStartFailed         GoalReasonCode = "runStartFailed"
	GoalReasonAwaitingInput          GoalReasonCode = "awaitingInput"
	GoalReasonTerminalOutcomeMissing GoalReasonCode = "terminalOutcomeMissing"
	GoalReasonRunNotCompleted        GoalReasonCode = "runNotCompleted"
	GoalReasonRunBudgetReached       GoalReasonCode = "runBudgetReached"
	GoalReasonCostBudgetReached      GoalReasonCode = "costBudgetReached"
	GoalReasonStepBudgetReached      GoalReasonCode = "stepBudgetReached"
	GoalReasonBlockedByModel         GoalReasonCode = "blockedByModel"
)

// GoalBudget is the opt-in cross-Run cap. A zero field is unbounded on that axis.
type GoalBudget struct {
	MaxRuns    int     `json:"maxRuns,omitempty"`
	MaxCostUSD float64 `json:"maxCostUsd,omitempty"`
	MaxSteps   int     `json:"maxSteps,omitempty"`
}

// GoalUsage is what the loop has spent so far.
type GoalUsage struct {
	Runs    int     `json:"runs"`
	CostUSD float64 `json:"costUsd"`
	Steps   int     `json:"steps"`
}

// StartGoalRequest — goals.start body.
type StartGoalRequest struct {
	SessionID string     `json:"sessionId"`
	Objective string     `json:"objective"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	Budget    GoalBudget `json:"budget,omitzero"`
}

// GoalRequest — goals.get / goals.stop / goals.resume body (keyed by session).
type GoalRequest struct {
	SessionID string `json:"sessionId"`
}
