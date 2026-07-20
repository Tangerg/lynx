// Package goaltool exposes the model-facing update_goal tool: the machine
// signal by which an autonomously-running agent declares its goal complete or
// blocked. The GoalDriver reads the resulting status after the turn — a model
// that merely says "done" in prose does NOT end the loop, only this tool does.
// The tool is offered only while a goal is active for the session (the resolver
// gates it), so it never appears in an ordinary, non-autonomous turn.
package goaltool

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
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

type tool struct {
	store goal.Store
	now   func() time.Time
}

// New builds the update_goal tool over store. It returns a nil tool and nil
// error when store is nil so the caller can simply omit it (Goal mode disabled).
// The session is read per-call off the turn's blackboard, so one instance serves
// every session.
func New(store goal.Store) (tools.Tool, error) {
	if store == nil {
		return nil, nil
	}
	return tools.New[updateArgs, string](
		tools.Config{Name: "update_goal", Description: description},
		(&tool{store: store, now: time.Now}).update,
	)
}

func (t *tool) update(ctx context.Context, a updateArgs) (string, error) {
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return "error: no active session — cannot update a goal", nil
	}
	g, ok, err := t.store.Get(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !ok || g.Status != goal.StatusActive {
		// Recoverable: the loop is not running, so there is nothing to end. The
		// model keeps working; it just cannot signal a goal that isn't active.
		return "No active goal for this session — nothing to update.", nil
	}
	now := t.now()
	switch a.Status {
	case string(goal.StatusComplete):
		g.Complete(now)
	case string(goal.StatusBlocked):
		if a.Reason == "" {
			return "Provide a concrete reason when marking the goal blocked.", nil
		}
		g.Block(a.Reason, now)
	default:
		return fmt.Sprintf("Unknown status %q — use \"complete\" or \"blocked\".", a.Status), nil
	}
	if err := t.store.Save(ctx, g); err != nil {
		return "", err
	}
	if g.Status == goal.StatusComplete {
		return "Goal marked complete — the autonomous loop will stop after this turn.", nil
	}
	return "Goal marked blocked — the loop will stop and surface your reason to the user.", nil
}
