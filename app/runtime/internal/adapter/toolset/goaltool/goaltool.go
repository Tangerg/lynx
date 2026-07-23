// Package goaltool exposes the model-facing update_goal tool: the machine
// signal by which an autonomously-running agent declares its goal complete or
// blocked. The GoalDriver reads the resulting status after the turn — a model
// that merely says "done" in prose does NOT end the loop, only this tool does.
// The tool is offered only while a goal is active for the session (the resolver
// gates it), so it never appears in an ordinary, non-autonomous turn.
package goaltool

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/tools"
)

const description = `Report the status of the autonomous goal you are pursuing.

Call this ONLY to end the autonomous loop, and only from a real machine state —
not because you feel done:
  - status="complete": the objective is fully achieved and verified (tests pass,
    no errors). Do NOT mark complete on a plan, a partial result, or an intention.
  - status="blocked": you cannot make further progress without the user — a
    missing decision, credential, or a dependency you have hit for several turns.
    Give a concrete reason.

While the goal is active and you have not called this, the loop keeps handing you
the next turn automatically — keep making progress toward the objective and pick
one bounded next step each turn.`

type updateArgs struct {
	Status string `json:"status" jsonschema:"required,enum=complete,enum=blocked" jsonschema_description:"complete = objective fully done and verified; blocked = stuck, needs the user."`
	Reason string `json:"reason,omitempty" jsonschema_description:"Why the goal is blocked. Required for blocked; ignored for complete."`
}

// State is the goal use case consumed by the model-facing tool. It deliberately
// exposes no persistence operations: terminal reporting and active-gating are
// the complete adapter needs.
type State interface {
	Active(ctx context.Context, sessionID string) (bool, error)
	Report(ctx context.Context, command goals.ReportCommand) (goals.ReportResult, error)
}

type tool struct {
	state State
}

// New builds the update_goal tool over the application state boundary. It returns a nil tool and nil
// error when state is nil so the caller can simply omit it (Goal mode disabled).
// The session is read per-call off the turn's blackboard, so one instance serves
// every session.
func New(state State) (tools.Tool, error) {
	if state == nil {
		return nil, nil
	}
	return tools.New[updateArgs, string](
		tools.Config{Name: "update_goal", Description: description},
		(&tool{state: state}).update,
	)
}

func (t *tool) update(ctx context.Context, a updateArgs) (string, error) {
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return "error: no active session — cannot update a goal", nil
	}
	leaseID, _ := turnctx.TurnGoalLease(ctx)
	result, err := t.state.Report(ctx, goals.ReportCommand{
		SessionID: sessionID,
		LeaseID:   leaseID,
		Status:    goal.Status(a.Status),
		Reason:    a.Reason,
	})
	if err != nil {
		return "", err
	}
	switch result {
	case goals.ReportApplied:
		if a.Status == string(goal.StatusComplete) {
			return "Goal marked complete — the autonomous loop will stop after this turn.", nil
		}
		return "Goal marked blocked — the loop will stop and surface your reason to the user.", nil
	case goals.ReportNoActiveGoal:
		return "No active goal for this session — nothing to update.", nil
	case goals.ReportSuperseded:
		return "This goal has been superseded since the run started — nothing to update.", nil
	case goals.ReportConflict:
		return "The goal changed concurrently — nothing to update.", nil
	case goals.ReportReasonRequired:
		return "Provide a concrete reason when marking the goal blocked.", nil
	case goals.ReportInvalidStatus:
		return "Unknown status \"" + a.Status + "\" — use \"complete\" or \"blocked\".", nil
	default:
		return "No active goal for this session — nothing to update.", nil
	}
}
