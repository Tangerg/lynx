package workflow

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// defaultLoopIterations bounds Loop when Config.MaxIterations is unset. Every
// iteration spawns a child that stays in the process tree until the host
// releases the root, so an unbounded loop grows framework-owned state — the
// same reason [runtime.DefaultMaxChildDepth] exists, and why this rail is not
// the caller's threshold to choose.
const defaultLoopIterations = 5

// LoopConfig configures a "run a sub-agent body repeatedly until
// Until returns true (or MaxIterations expires)" workflow. Each
// iteration runs Body via [runtime.Engine.RunChild] — a child process
// with a CLEAN blackboard seeded only with the typed input. This
// branch isolation is essential: without it, the orchestrator's
// accumulated Out bindings would leak into the body's blackboard, and
// the body's "produce Out" goal would be considered already satisfied
// (so the body would short-circuit without doing work).
//
// Because each iteration starts clean, the body sub-agent **cannot
// read its own prior outputs**. If iteration-aware behavior is
// needed, encode it externally — closure state, an injected dependency,
// or fold the previous Out into the typed In type so the
// orchestrator's typed wrapper feeds it back in (the next iteration's
// In is resolved from the parent blackboard via type-based binding,
// where every iteration's Out has been bound by the typed action
// wrapper).
//
// Compare to [RepeatUntilConfig]: that one's Task is an inline closure
// (action level); LoopConfig.Body is a full sub-agent (agent
// level), so Body can have its own LLM tool loop, sub-actions, etc.
// Use [RepeatUntilConfig] for "loop a single function"; use Loop
// for "loop a whole agent".
type LoopConfig[In, Out any] struct {
	// Name names the produced agent + its goal + the iteration's
	// computed condition. Required; surrounding whitespace is invalid.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxIterations bounds the loop; zero defaults to 5 and negative values are
	// invalid. The workflow always runs Body at least once (the action is Repeatable, so
	// planner reschedules until Until says stop).
	MaxIterations int

	// Body is the per-iteration sub-agent. It receives In on its
	// blackboard each iteration and is expected to bind a fresh Out.
	// Required.
	Body *core.Agent

	// Until inspects the loop input + the latest body output and
	// returns true to stop the loop. Required.
	Until func(ctx context.Context, input In, latest Out) bool
}

// Loop compiles config into a deployable agent. The compiled agent
// has one Repeatable=true action ("{Name}-iter") that runs Body once and
// records the result on the parent blackboard via [History][Out];
// after each run the runtime re-evaluates the "{Name}_done" computed
// condition. When Until or MaxIterations triggers, the goal (which
// preconditions on it) is satisfied and the loop terminates; otherwise
// GOAP re-plans and runs the action again.
//
// Mirrors [RepeatUntil]'s mechanics — same single-action +
// computed-condition + History pattern — substituting "run a sub-agent"
// for "call a closure".
//
// Returns an error on a nil engine, missing or whitespace-padded Name, nil or
// structurally invalid Body, nil Until, or negative MaxIterations. Static
// validation finishes before Body is deployed.
func Loop[In, Out any](
	ctx context.Context,
	engine *runtime.Engine,
	config LoopConfig[In, Out],
) (*core.Agent, error) {
	if engine == nil {
		return nil, errors.New("workflow.Loop: engine must not be nil")
	}
	if err := validateName("Loop", config.Name); err != nil {
		return nil, err
	}
	if config.Body == nil {
		return nil, errors.New("workflow.Loop: Body must not be nil")
	}
	if config.Until == nil {
		return nil, errors.New("workflow.Loop: Until must not be nil")
	}
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("workflow.Loop: MaxIterations %d must not be negative", config.MaxIterations)
	}
	if err := config.Body.Validate(); err != nil {
		return nil, fmt.Errorf("workflow.Loop: Body %q is invalid: %w", config.Body.Name(), err)
	}
	bodyDeployment, err := engine.Deploy(ctx, config.Body)
	if err != nil {
		return nil, fmt.Errorf("workflow.Loop: deploy Body %q: %w", config.Body.Name(), err)
	}
	bodyName := bodyDeployment.Ref().Name
	maxIterations := cmp.Or(config.MaxIterations, defaultLoopIterations)

	doneKey := config.Name + "_done"
	historyState := core.NewBinding[History[Out]](config.Name + historyStateSuffix)

	return compileRepeatWorkflow(repeatWorkflowConfig[In, Out, History[Out]]{
		name:          config.Name,
		description:   config.Description,
		actionName:    config.Name + "-iter",
		doneKey:       doneKey,
		maxIterations: maxIterations,
		stateBinding:  historyState,
		newState:      func() History[Out] { return History[Out]{} },
		count:         History[Out].Count,
		run: func(ctx context.Context, process *core.ProcessContext, input In, history History[Out]) (Out, History[Out], error) {
			var zero Out
			child, err := engine.RunChild(ctx, bodyDeployment, input)
			if err != nil {
				return zero, history, fmt.Errorf("iteration %d: %w", history.Count(), err)
			}
			if err := child.CompletionError(); err != nil {
				return zero, history, fmt.Errorf("iteration %d (%s): %w", history.Count(), bodyName, err)
			}

			output, ok := core.Result[Out](child)
			if !ok {
				return zero, history, fmt.Errorf("iteration %d (%s) produced no %T", history.Count(), bodyName, zero)
			}
			return output, history.withAttempt(output), nil
		},
		until: func(ctx context.Context, input In, history History[Out]) bool {
			last, ok := history.Last()
			return ok && config.Until(ctx, input, last)
		},
	}), nil
}
