// Goal tools expose the root Agent's model-facing autonomous Goal
// controls. The adapters depend only on narrow application use cases: they do
// not know how Goals are persisted, scheduled, or joined to Run admission.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	goalstate "github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
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
	MaxRuns    int     `json:"max_runs,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum autonomous Runs. Zero or omitted means no Run limit."`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum accumulated model cost in USD. Zero or omitted means no cost limit."`
	MaxSteps   int     `json:"max_steps,omitempty" jsonschema:"minimum=0" jsonschema_description:"Maximum accumulated model steps. Zero or omitted means no step limit."`
}

type getArgs struct{}

type reportArgs struct {
	Outcome string  `json:"outcome" jsonschema:"required,enum=completed,enum=blocked" jsonschema_description:"completed = the whole objective is achieved and verified; blocked = progress requires the user or an external state change."`
	Reason  *string `json:"reason,omitempty" jsonschema_description:"Concrete blocker and what must change. Required for blocked; omit for completed."`
}

// GoalReader is get_goal's complete consumer view.
type GoalReader interface {
	Current(ctx context.Context, sessionID string) (goalstate.Goal, bool, error)
}

// GoalOutcomeReporter is report_goal_outcome's complete consumer view.
type GoalOutcomeReporter interface {
	Report(ctx context.Context, command goals.ReportCommand) (goals.ReportResult, error)
}

// GoalStarter is the one lifecycle operation create_goal needs. The Driver owns
// admission waiting and loop lifetime; the tool does not reproduce either.
type GoalStarter interface {
	Start(ctx context.Context, sessionID, objective string, selection modelref.Selection, budget goalstate.Budget, capabilities run.Capabilities) (goalstate.Goal, error)
}

type creator struct{ goals GoalStarter }
type getter struct{ goals GoalReader }
type outcomeReporter struct{ goals GoalOutcomeReporter }

type goalResult struct {
	Goal    *goalView `json:"goal"`
	Message string    `json:"message,omitempty"`
}

// goalView deliberately excludes the Goal incarnation and persistence revision.
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
	MaxRuns    int     `json:"max_runs,omitempty"`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty"`
	MaxSteps   int     `json:"max_steps,omitempty"`
}

type usageView struct {
	Runs    int     `json:"runs"`
	CostUSD float64 `json:"cost_usd"`
	Steps   int     `json:"steps"`
}

// NewCreate builds create_goal. It is assembled after the Goal Driver because
// starting a Goal consumes the Run coordinator that is built later than the
// static tool environment.
func NewCreate(starter GoalStarter) (toolcontract.Tool, error) {
	if starter == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[createArgs, goalResult](
		toolcontract.FuncConfig{Name: tool.CreateGoal, Description: createDescription},
		(&creator{goals: starter}).create,
	)
}

// NewGet builds get_goal over the read-only side of Goal state.
func NewGet(reader GoalReader) (toolcontract.Tool, error) {
	if reader == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[getArgs, goalResult](
		toolcontract.FuncConfig{Name: tool.GetGoal, Description: getDescription},
		(&getter{goals: reader}).get,
	)
}

// NewReport builds report_goal_outcome, the active Goal loop's terminal signal.
func NewReport(reporter GoalOutcomeReporter) (toolcontract.Tool, error) {
	if reporter == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[reportArgs, string](
		toolcontract.FuncConfig{Name: tool.ReportGoalOutcome, Description: reportDescription},
		(&outcomeReporter{goals: reporter}).report,
	)
}

func (t *creator) create(ctx context.Context, args createArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return goalResult{Message: "No active session; a Goal must belong to a session."}, nil
	}
	objective := strings.TrimSpace(args.Objective)
	if objective == "" {
		return goalResult{Message: "Provide a non-empty autonomous objective."}, nil
	}
	var budget goalstate.Budget
	if args.Budget != nil {
		budget = goalstate.Budget{
			MaxRuns:    args.Budget.MaxRuns,
			MaxCostUSD: args.Budget.MaxCostUSD,
			MaxSteps:   args.Budget.MaxSteps,
		}
	}
	capabilities, _ := executionctx.RunCapabilities(ctx)
	g, err := t.goals.Start(ctx, sessionID, objective, modelref.Selection{}, budget, capabilities)
	if err != nil {
		switch {
		case errors.Is(err, goals.ErrGoalActive):
			return goalResult{Message: "An active Goal already exists for this session. Inspect it with get_goal before deciding what to do."}, nil
		case errors.Is(err, goals.ErrNoSession):
			return goalResult{Message: "The current session no longer exists; no Goal was created."}, nil
		case errors.Is(err, goals.ErrUnavailable):
			return goalResult{Message: "Autonomous Goals are unavailable."}, nil
		case errors.Is(err, goals.ErrClosed):
			return goalResult{Message: "The service is shutting down; no Goal was created."}, nil
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

func (t *getter) get(ctx context.Context, _ getArgs) (goalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return goalResult{Message: "No active session; there is no session Goal to inspect."}, nil
	}
	g, ok, err := t.goals.Current(ctx, sessionID)
	if err != nil {
		return goalResult{}, err
	}
	if !ok {
		return goalResult{Message: "No Goal exists for this session."}, nil
	}
	view := viewOf(g)
	return goalResult{Goal: &view}, nil
}

func (t *outcomeReporter) report(ctx context.Context, args reportArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "No active session; cannot report a Goal outcome.", nil
	}
	outcome := goalstate.StatusComplete
	reason := ""
	if args.Outcome == "blocked" {
		outcome = goalstate.StatusBlocked
		if args.Reason != nil {
			reason = strings.TrimSpace(*args.Reason)
		}
		if reason == "" {
			return "Provide a concrete reason when reporting a blocked Goal.", nil
		}
	} else if args.Reason != nil {
		return "Omit reason when reporting a completed Goal.", nil
	}
	incarnationID, _ := executionctx.GoalIncarnationID(ctx)
	result, err := t.goals.Report(ctx, goals.ReportCommand{
		SessionID:     sessionID,
		IncarnationID: incarnationID,
		Outcome:       outcome,
		Reason:        reason,
	})
	if err != nil {
		return "", err
	}
	if !result.Valid() {
		return "", fmt.Errorf("goal tool received invalid report result %q", result)
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
	case goals.ReportInvalidOutcome:
		return "Invalid Goal outcome; use completed or blocked.", nil
	default:
		return "", errors.New("goal tool report result has no presentation")
	}
}

func viewOf(g goalstate.Goal) goalView {
	return goalView{
		SessionID: g.SessionID,
		Objective: g.Objective,
		Status:    string(g.Status),
		Reason:    reasonText(g.Reason),
		Provider:  g.ModelSelection.Provider(),
		Model:     g.ModelSelection.Model(),
		Budget: budgetView{
			MaxRuns:    g.Budget.MaxRuns,
			MaxCostUSD: g.Budget.MaxCostUSD,
			MaxSteps:   g.Budget.MaxSteps,
		},
		Usage: usageView{
			Runs:    g.Used.Runs,
			CostUSD: g.Used.CostUSD,
			Steps:   g.Used.Steps,
		},
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

// reasonText is adapter-owned presentation. Keeping it here avoids making
// application or domain packages depend on model-facing prose.
func reasonText(reason goalstate.Reason) string {
	switch reason.Code {
	case goalstate.ReasonNone:
		return ""
	case goalstate.ReasonStoppedByUser:
		return "stopped by the user"
	case goalstate.ReasonRuntimeRestarted:
		return "the service restarted; resume to continue"
	case goalstate.ReasonRunStartFailed:
		return "could not start the next Run"
	case goalstate.ReasonAwaitingInput:
		return "the Run is waiting for user input"
	case goalstate.ReasonTerminalOutcomeMissing:
		return "the Run ended without a terminal outcome"
	case goalstate.ReasonRunNotCompleted:
		if reason.Detail == "" {
			return "the Run ended before completing the Goal"
		}
		return "the Run ended: " + reason.Detail
	case goalstate.ReasonRunBudgetReached:
		return "reached the Run budget"
	case goalstate.ReasonCostBudgetReached:
		return "reached the cost budget"
	case goalstate.ReasonStepBudgetReached:
		return "reached the step budget"
	case goalstate.ReasonBlockedByModel:
		if reason.Detail != "" {
			return reason.Detail
		}
		return "the model reported that it is blocked"
	default:
		return "the Goal stopped"
	}
}
