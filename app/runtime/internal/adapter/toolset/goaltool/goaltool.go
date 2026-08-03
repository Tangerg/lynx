// Package goaltool exposes the root Agent's model-facing autonomous Goal
// controls. The adapters depend only on narrow application use cases: they do
// not know how Goals are persisted, scheduled, or joined to Run admission.
package goaltool

import (
	"context"
	"errors"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

const createDescription = `Create and start a persistent autonomous Goal for the current session.

Call this only when the user explicitly asks for work to continue autonomously
across Runs. Do not infer Goal intent from an ordinary coding request; use
set_plan for the current request instead. The Goal starts after the current Run
releases the session and keeps launching bounded next Runs until it reports an
outcome, reaches an explicit budget, or the user stops it.

Copy the user's desired end state into objective. Omit budget unless the user
explicitly requested a limit; omitted limits are unbounded.`

const getDescription = `Get the current session's autonomous Goal, including its objective, status,
stop reason, optional budget, accumulated usage, model selection, and timestamps.
Returns goal=null when no Goal exists. Use this before creating a Goal when its
current state is uncertain.`

const reportDescription = `Report the terminal outcome of the autonomous Goal you are currently pursuing.

Call this only from an autonomous Goal Run:
  - outcome="completed" only when the entire objective is achieved and verified;
  - outcome="blocked" only when progress genuinely requires user input or an
    external state change, with a concrete reason.

Do not report completion for a plan, partial result, intention, or because one
Run ended. While no terminal outcome is reported, the Goal loop supplies the
next Run automatically.`

type createArgs struct {
	Objective string        `json:"objective" jsonschema:"required,minLength=1" jsonschema_description:"The complete end state to achieve autonomously, in natural language."`
	Budget    *createBudget `json:"budget,omitempty" jsonschema_description:"Optional cross-Run limits. Omit unless the user explicitly requested a limit."`
}

type createBudget struct {
	MaxTurns   int     `json:"max_turns,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum autonomous Runs. Zero or omitted means no turn limit."`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum accumulated model cost in USD. Zero or omitted means no cost limit."`
	MaxSteps   int     `json:"max_steps,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum accumulated model steps. Zero or omitted means no step limit."`
}

type getArgs struct{}

type reportArgs struct {
	Outcome string `json:"outcome" jsonschema:"required,enum=completed,enum=blocked" jsonschema_description:"completed = the whole objective is achieved and verified; blocked = progress requires the user or an external state change."`
	Reason  string `json:"reason,omitempty" jsonschema_description:"Concrete blocker and what must change. Required for blocked; omit for completed."`
}

// Reader is get_goal's complete consumer view.
type Reader interface {
	Get(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// ActiveReader is the resolver gate's complete consumer view.
type ActiveReader interface {
	Active(ctx context.Context, sessionID string) (bool, error)
}

// Reporter is report_goal_outcome's complete consumer view.
type Reporter interface {
	Report(ctx context.Context, command goals.ReportCommand) (goals.ReportResult, error)
}

// State combines the three Goal-state capabilities required to assemble the
// complete model-facing tool family. Individual constructors consume the
// single-method interfaces above.
type State interface {
	Reader
	ActiveReader
	Reporter
}

// Starter is the one lifecycle operation create_goal needs. The Driver owns
// admission waiting and loop lifetime; the tool does not reproduce either.
type Starter interface {
	Start(ctx context.Context, sessionID, objective string, selection modelref.Selection, budget goal.Budget) (goal.Goal, error)
}

type createTool struct{ goals Starter }
type getTool struct{ goals Reader }
type reportTool struct{ goals Reporter }

type goalResult struct {
	Goal    *goalView `json:"goal"`
	Message string    `json:"message,omitempty"`
}

// goalView deliberately excludes the loop lease and persistence revision.
// Those are ownership mechanics, not facts the model can act on.
type goalView struct {
	SessionID string     `json:"session_id"`
	Objective string     `json:"objective"`
	Status    string     `json:"status"`
	Reason    string     `json:"reason,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	Budget    budgetView `json:"budget"`
	Usage     usageView  `json:"usage"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type budgetView struct {
	MaxTurns   int     `json:"max_turns,omitempty"`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty"`
	MaxSteps   int     `json:"max_steps,omitempty"`
}

type usageView struct {
	Turns   int     `json:"turns"`
	CostUSD float64 `json:"cost_usd"`
	Steps   int     `json:"steps"`
}

// NewCreate builds create_goal. It is assembled after the Goal Driver because
// starting a Goal consumes the Run coordinator that is built later than the
// static tool environment.
func NewCreate(starter Starter) (toolcontract.Tool, error) {
	if starter == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[createArgs, goalResult](
		toolcontract.FuncConfig{Name: "create_goal", Description: createDescription},
		(&createTool{goals: starter}).create,
	)
}

// NewGet builds get_goal over the read-only side of Goal state.
func NewGet(reader Reader) (toolcontract.Tool, error) {
	if reader == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[getArgs, goalResult](
		toolcontract.FuncConfig{Name: "get_goal", Description: getDescription},
		(&getTool{goals: reader}).get,
	)
}

// NewReport builds report_goal_outcome, the active Goal loop's terminal signal.
func NewReport(reporter Reporter) (toolcontract.Tool, error) {
	if reporter == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[reportArgs, string](
		toolcontract.FuncConfig{Name: "report_goal_outcome", Description: reportDescription},
		(&reportTool{goals: reporter}).report,
	)
}

func (t *createTool) create(ctx context.Context, args createArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return goalResult{Message: "No active session; a Goal must belong to a session."}, nil
	}
	objective := strings.TrimSpace(args.Objective)
	if objective == "" {
		return goalResult{Message: "Provide a non-empty autonomous objective."}, nil
	}
	var budget goal.Budget
	if args.Budget != nil {
		budget = goal.Budget{
			MaxTurns:   args.Budget.MaxTurns,
			MaxCostUSD: args.Budget.MaxCostUSD,
			MaxSteps:   args.Budget.MaxSteps,
		}
	}
	g, err := t.goals.Start(ctx, sessionID, objective, modelref.Selection{}, budget)
	if err != nil {
		switch {
		case errors.Is(err, goals.ErrGoalActive):
			return goalResult{Message: "An active Goal already exists for this session. Inspect it with get_goal before deciding what to do."}, nil
		case errors.Is(err, goals.ErrNoSession):
			return goalResult{Message: "The current session no longer exists; no Goal was created."}, nil
		case errors.Is(err, goals.ErrUnavailable):
			return goalResult{Message: "Autonomous Goals are unavailable in this runtime."}, nil
		case errors.Is(err, goals.ErrClosed):
			return goalResult{Message: "The runtime is shutting down; no Goal was created."}, nil
		case errors.Is(err, goals.ErrGoalConflict):
			return goalResult{Message: "The Goal changed concurrently. Inspect the current state with get_goal before retrying."}, nil
		default:
			return goalResult{}, err
		}
	}
	view := viewOf(g)
	return goalResult{
		Goal:    &view,
		Message: "Goal created. Its autonomous loop will begin after the current Run releases the session.",
	}, nil
}

func (t *getTool) get(ctx context.Context, _ getArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return goalResult{Message: "No active session; there is no session Goal to inspect."}, nil
	}
	g, ok, err := t.goals.Get(ctx, sessionID)
	if err != nil {
		return goalResult{}, err
	}
	if !ok {
		return goalResult{Message: "No Goal exists for this session."}, nil
	}
	view := viewOf(g)
	return goalResult{Goal: &view}, nil
}

func (t *reportTool) report(ctx context.Context, args reportArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "No active session; cannot report a Goal outcome.", nil
	}
	status := goal.StatusComplete
	reason := ""
	if args.Outcome == "blocked" {
		status = goal.StatusBlocked
		reason = strings.TrimSpace(args.Reason)
		if reason == "" {
			return "Provide a concrete reason when reporting a blocked Goal.", nil
		}
	}
	leaseID, _ := executionctx.GoalLeaseID(ctx)
	result, err := t.goals.Report(ctx, goals.ReportCommand{
		SessionID: sessionID,
		LeaseID:   leaseID,
		Status:    status,
		Reason:    reason,
	})
	if err != nil {
		return "", err
	}
	switch result {
	case goals.ReportApplied:
		if args.Outcome == "completed" {
			return "Goal outcome reported as completed. The autonomous loop will stop after this Run.", nil
		}
		return "Goal outcome reported as blocked. The loop will stop and surface the reason to the user.", nil
	case goals.ReportNoActiveGoal:
		return "No active Goal exists for this session; no outcome was reported.", nil
	case goals.ReportSuperseded:
		return "This Run belongs to a superseded Goal; no outcome was reported.", nil
	case goals.ReportConflict:
		return "The Goal changed concurrently; inspect it with get_goal before reporting an outcome.", nil
	case goals.ReportReasonRequired:
		return "Provide a concrete reason when reporting a blocked Goal.", nil
	case goals.ReportInvalidStatus:
		return "Invalid Goal outcome; use completed or blocked.", nil
	default:
		return "No active Goal exists for this session; no outcome was reported.", nil
	}
}

func viewOf(g goal.Goal) goalView {
	return goalView{
		SessionID: g.SessionID,
		Objective: g.Objective,
		Status:    string(g.Status),
		Reason:    reasonText(g.Reason),
		Provider:  g.ModelSelection.Provider(),
		Model:     g.ModelSelection.Model(),
		Budget: budgetView{
			MaxTurns:   g.Budget.MaxTurns,
			MaxCostUSD: g.Budget.MaxCostUSD,
			MaxSteps:   g.Budget.MaxSteps,
		},
		Usage: usageView{
			Turns:   g.Used.Turns,
			CostUSD: g.Used.CostUSD,
			Steps:   g.Used.Steps,
		},
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

// reasonText is adapter-owned presentation. Keeping it here avoids making
// application or domain packages depend on model-facing prose.
func reasonText(reason goal.Reason) string {
	switch reason.Cause {
	case goal.ReasonNone:
		return ""
	case goal.ReasonStoppedByUser:
		return "stopped by the user"
	case goal.ReasonRuntimeRestarted:
		return "the runtime restarted; resume to continue"
	case goal.ReasonRunStartFailed:
		return "could not start the next Run"
	case goal.ReasonAwaitingInput:
		return "the Run is waiting for user input"
	case goal.ReasonTerminalOutcomeMissing:
		return "the Run ended without a terminal outcome"
	case goal.ReasonRunNotCompleted:
		if reason.Detail == "" {
			return "the Run ended before completing the Goal"
		}
		return "the Run ended: " + reason.Detail
	case goal.ReasonTurnBudgetReached:
		return "reached the turn budget"
	case goal.ReasonCostBudgetReached:
		return "reached the cost budget"
	case goal.ReasonStepBudgetReached:
		return "reached the step budget"
	case goal.ReasonBlockedByModel:
		if reason.Detail != "" {
			return reason.Detail
		}
		return "the model reported that it is blocked"
	default:
		return "the Goal stopped"
	}
}
