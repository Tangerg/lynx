package protocol

import (
	"context"
	"time"
)

// Goals is the goals.* method group — Goal mode, an autonomous execution loop
// that drives runs toward an objective until the model signals it complete or
// blocked (via the update_goal tool), an opt-in cross-turn budget is spent, or
// the user stops it. A session has at most one goal. Starting a goal is the
// explicit opt-in gate; while it runs, the runtime launches runs back-to-back on
// its own instead of the user driving each turn.
type Goals interface {
	// StartGoal opens a goal for the session and begins driving it. sessionId +
	// objective are required; provider/model pair the model each turn runs
	// against; budget caps the loop (all-zero = uncapped, an explicit choice).
	// Fails if the session already has an actively-driving goal.
	StartGoal(ctx context.Context, in StartGoalRequest) (*Goal, error)
	// GetGoal returns the session's goal, or a nil result when it has none.
	GetGoal(ctx context.Context, in GoalRequest) (*Goal, error)
	// StopGoal pauses the session's goal and stops launching runs. The in-flight
	// run (if any) finishes on its own.
	StopGoal(ctx context.Context, in GoalRequest) (*Goal, error)
	// ResumeGoal returns a paused or blocked goal to active and drives it again.
	ResumeGoal(ctx context.Context, in GoalRequest) (*Goal, error)
}

// Goal is one session's autonomous objective and loop state (API.md §7.14).
// status is active | paused | blocked; a completed goal is cleared, so it never
// appears here.
type Goal struct {
	SessionID string     `json:"sessionId"`
	Objective string     `json:"objective"`
	Status    string     `json:"status"`
	Reason    string     `json:"reason,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	Budget    GoalBudget `json:"budget"`
	Used      GoalUsage  `json:"used"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// GoalBudget is the opt-in cross-turn cap. A zero field is unbounded on that axis.
type GoalBudget struct {
	MaxTurns   int     `json:"maxTurns,omitempty"`
	MaxCostUsd float64 `json:"maxCostUsd,omitempty"`
	MaxSteps   int     `json:"maxSteps,omitempty"`
}

// GoalUsage is what the loop has spent so far.
type GoalUsage struct {
	Turns   int     `json:"turns"`
	CostUsd float64 `json:"costUsd"`
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
