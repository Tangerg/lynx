package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// DefaultLoopIterations bounds Loop when Config.MaxIterations is unset.
const DefaultLoopIterations = 5

// LoopConfig configures a "run a sub-agent body repeatedly until
// Until returns true (or MaxIterations expires)" workflow. Each
// iteration runs Body via [runtime.RunChildIsolated] — a child process
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
	// computed condition. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxIterations bounds the loop; <=0 defaults to 5. The workflow
	// always runs Body at least once (the action is Repeatable, so
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
// Returns an error on missing Name, nil Body, or nil Until.
func Loop[In, Out any](
	engine *runtime.Engine,
	config LoopConfig[In, Out],
) (*core.Agent, error) {
	if engine == nil {
		return nil, errors.New("workflow.Loop: engine must not be nil")
	}
	if config.Name == "" {
		return nil, errors.New("workflow.Loop: Name must not be empty")
	}
	if config.Body == nil {
		return nil, errors.New("workflow.Loop: Body must not be nil")
	}
	if config.Until == nil {
		return nil, errors.New("workflow.Loop: Until must not be nil")
	}
	bodyDeployment, err := engine.Deploy(config.Body)
	if err != nil {
		return nil, fmt.Errorf("workflow.Loop: deploy Body %q: %w", config.Body.Name(), err)
	}
	bodyName := bodyDeployment.Ref().Name
	maxIterations := config.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultLoopIterations
	}

	// Condition keys must not contain ':' — the determiner reserves
	// that for type-binding keys. Use '_' as the separator.
	doneKey := config.Name + "_done"
	historyState := core.NewBinding[*History[Out]](config.Name + historyStateSuffix)
	inputState := core.NewBinding[loopInput[In]](config.Name + inputStateSuffix)

	doneCondition := core.NewCondition(doneKey, func(ctx context.Context, env *core.ConditionEnv) core.Truth {
		history, ok := core.Last[*History[Out]](env.Blackboard)
		if !ok {
			return core.False
		}
		last, ok := history.Last()
		if !ok {
			return core.False
		}
		if history.Count() >= maxIterations {
			return core.True
		}
		// Read the ORIGINAL loop input via loopInput, not core.Last[In]: when
		// In==Out the per-iteration outputs would shadow the input.
		var input In
		if original, ok := core.Last[loopInput[In]](env.Blackboard); ok {
			input = original.Value
		}
		if config.Until(ctx, input, last) {
			return core.True
		}
		return core.False
	})

	iter := core.NewAction[In, Out](
		config.Name+"-iter",
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			var zero Out

			history, ok := core.Last[*History[Out]](process.Blackboard())
			if !ok {
				history = &History[Out]{}
				process.Blackboard().Store(historyState.Name, history)
				// First iteration: `in` is the original input — stash it so later
				// iterations feed the SAME input to Body even when In==Out (else the
				// framework binds `in` from the latest Out).
				process.Blackboard().Store(inputState.Name, loopInput[In]{Value: input})
			} else if original, ok := core.Last[loopInput[In]](process.Blackboard()); ok {
				input = original.Value
			}

			child, err := engine.RunChildIsolated(ctx, bodyDeployment, input)
			if err != nil {
				return zero, fmt.Errorf("iteration %d: %w", history.Count(), err)
			}
			if err := child.TerminalError(); err != nil {
				return zero, fmt.Errorf("iteration %d (%s): %w", history.Count(), bodyName, err)
			}

			output, ok := core.Result[Out](child)
			if !ok {
				return zero, fmt.Errorf("iteration %d (%s) produced no %T", history.Count(), bodyName, zero)
			}
			history.record(output)
			return output, nil
		},
		core.ActionConfig{
			Description: "loop body iteration (sub-agent run)",
			Repeatable:  true,
			Effects:     []string{doneKey},
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:         config.Name,
		Description:  config.Description,
		Actions:      []core.Action{iter},
		Conditions:   []core.Condition{doneCondition},
		DurableState: []core.Binding{historyState, inputState},
		Goals:        []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: config.Name, Description: "produce acceptable " + core.TypeName[Out](), Preconditions: []string{doneKey}})},
	}), nil
}
