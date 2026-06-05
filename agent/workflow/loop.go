package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// LoopConfig configures a "run a sub-agent body repeatedly until
// Until returns true (or MaxIterations expires)" workflow. Each
// iteration runs Body via [runtime.SpawnChildFresh] — a child process
// with a CLEAN blackboard seeded only with the typed input. This
// branch isolation is essential: without it, the orchestrator's
// accumulated Out bindings would leak into the body's blackboard, and
// the body's "produce Out" goal would be considered already satisfied
// (so the body would short-circuit without doing work).
//
// Because each iteration starts clean, the body sub-agent **cannot
// read its own prior outputs**. If iteration-aware behavior is
// needed, encode it externally — closure state, an injected service,
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
	// always runs Body at least once (the action is CanRerun, so
	// planner reschedules until Until says stop).
	MaxIterations int

	// Body is the per-iteration sub-agent. It receives In on its
	// blackboard each iteration and is expected to bind a fresh Out.
	// Required.
	Body *core.Agent

	// Until inspects the loop input + the latest body output and
	// returns true to stop the loop. Required.
	Until func(ctx context.Context, in In, last Out) bool
}

// Loop compiles spec into a deployable agent. The compiled agent
// has one CanRerun=true action ("{Name}-iter") that runs Body once and
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
	platform *runtime.Platform,
	spec LoopConfig[In, Out],
) (*core.Agent, error) {
	if platform == nil {
		return nil, errors.New("workflow.Loop: platform must not be nil")
	}
	if spec.Name == "" {
		return nil, errors.New("workflow.Loop: Name must not be empty")
	}
	if spec.Body == nil {
		return nil, errors.New("workflow.Loop: Body must not be nil")
	}
	if spec.Until == nil {
		return nil, errors.New("workflow.Loop: Until must not be nil")
	}
	maxIter := spec.MaxIterations
	if maxIter <= 0 {
		maxIter = 5
	}

	// Condition keys must not contain ':' — the determiner reserves
	// that for type-binding keys. Use '_' as the separator.
	doneKey := spec.Name + "_done"

	doneCondition := core.NewCondition(doneKey, func(ctx context.Context, oc *core.ConditionEnv) core.Determination {
		history, ok := core.Last[*History[Out]](oc.Blackboard)
		if !ok {
			return core.False
		}
		last, ok := history.Last()
		if !ok {
			return core.False
		}
		if history.Count() >= maxIter {
			return core.True
		}
		in, _ := core.Last[In](oc.Blackboard)
		if spec.Until(ctx, in, last) {
			return core.True
		}
		return core.False
	})

	iter := core.NewAction[In, Out](
		spec.Name+"-iter",
		func(ctx context.Context, pc *core.ProcessContext, in In) (Out, error) {
			var zero Out

			history, ok := core.Last[*History[Out]](pc.Blackboard)
			if !ok {
				history = &History[Out]{}
				pc.Blackboard.Bind(history)
			}

			child, err := runtime.SpawnChildFresh(ctx, platform, spec.Body, in)
			if err != nil {
				return zero, fmt.Errorf("iteration %d: %w", history.Count(), err)
			}
			if err := child.TerminalError(); err != nil {
				return zero, fmt.Errorf("iteration %d (%s): %w", history.Count(), spec.Body.Name, err)
			}

			out, ok := core.ResultOfType[Out](child)
			if !ok {
				return zero, fmt.Errorf("iteration %d (%s) produced no %T", history.Count(), spec.Body.Name, zero)
			}
			history.record(out)
			return out, nil
		},
		core.ActionConfig{
			Description: "loop body iteration (sub-agent run)",
			CanRerun:    true,
			Post:        []string{doneKey},
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        spec.Name,
		Description: spec.Description,
		Actions:     []core.Action{iter},
		Conditions:  []core.Condition{doneCondition},
		Goals: []*core.Goal{core.GoalProducing[Out](core.Goal{
			Name:        spec.Name,
			Description: "produce acceptable " + core.TypeName[Out](),
			Pre:         []string{doneKey},
		})},
	}), nil
}
