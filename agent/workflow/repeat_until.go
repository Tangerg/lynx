package workflow

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
)

// RepeatUntilConfig configures a "loop a task until the result is
// acceptable" workflow. Each iteration runs Task to produce a fresh
// Out; Accept then inspects the latest attempt (with full History)
// to decide whether the workflow should stop.
//
// The MaxIterations cap forces termination after that many attempts
// even when Accept never returns true — the workflow then yields the
// last attempt as the final result.
type RepeatUntilConfig[In, Out any] struct {
	// Name names the produced agent + its goal + the iteration's
	// computed condition. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxIterations bounds the loop. <=0 defaults to 3. The workflow always runs Task
	// at least once.
	MaxIterations int

	// Task is the per-iteration body. It receives the loop input In,
	// the running history (so it can inspect prior attempts), and
	// returns the next attempt. Required.
	Task func(ctx context.Context, process *core.ProcessContext, input In, history *History[Out]) (Out, error)

	// Accept inspects the latest attempt and returns true to stop
	// the loop. Receives the loop input, the latest Out, and the
	// full history (latest is also history.Last()). Required.
	Accept func(ctx context.Context, input In, latest Out, history *History[Out]) bool
}

// RepeatUntil compiles config into a deployable [*core.Agent].
//
// The agent has one action — "{Name}-task" — that produces Out and
// is flagged [ActionConfig.Repeatable] so the planner can pick it
// repeatedly. After every run the runtime re-evaluates the
// "{Name}-acceptable" computed condition: when true, the goal
// (which preconditions on it) is satisfied and the loop terminates;
// when false, GOAP re-plans and runs the task again. The
// MaxIterations cap forces the condition to true regardless once
// reached, so a never-accepting Accept can't loop forever.
//
// History is bound on the blackboard at first iteration and
// mutated (via its record method) on each subsequent run, so
// user-supplied Task / Accept callbacks always see the running record.
//
// Returns an error on missing Name, nil Task, or nil Accept.
func RepeatUntil[In, Out any](config RepeatUntilConfig[In, Out]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.RepeatUntil: Name must not be empty")
	}
	if config.Task == nil {
		return nil, errors.New("workflow.RepeatUntil: Task must not be nil")
	}
	if config.Accept == nil {
		return nil, errors.New("workflow.RepeatUntil: Accept must not be nil")
	}
	maxIterations := config.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 3
	}

	// Condition keys must not contain ':' — the determiner reserves
	// that for type-binding keys. Use '_' as the separator.
	acceptKey := config.Name + "_acceptable"

	acceptCondition := core.NewCondition(acceptKey, func(ctx context.Context, env *core.ConditionEnv) core.Truth {
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
			input = original.value
		}
		if config.Accept(ctx, input, last, history) {
			return core.True
		}
		return core.False
	})

	task := core.NewAction[In, Out](
		config.Name+"-task",
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			history, ok := core.Last[*History[Out]](process.Blackboard())
			if !ok {
				history = &History[Out]{}
				process.Blackboard().Bind(history)
				// First iteration: `in` IS the original input (no Out bound yet to
				// shadow it). Stash it so later iterations + Accept recover it even
				// when In==Out.
				process.Blackboard().Bind(loopInput[In]{value: input})
			} else if original, ok := core.Last[loopInput[In]](process.Blackboard()); ok {
				// Later iterations: the framework binds `in` from Last[In], which is
				// the latest Out when In==Out — restore the original.
				input = original.value
			}
			output, err := config.Task(ctx, process, input, history)
			if err != nil {
				var zero Out
				return zero, err
			}
			history.record(output)
			return output, nil
		},
		core.ActionConfig{
			Description: "loop body — produces a candidate Out",
			Repeatable:  true,
			Effects:     []string{acceptKey},
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        config.Name,
		Description: config.Description,
		Actions:     []core.Action{task},
		Conditions:  []core.Condition{acceptCondition},
		Goals:       []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: config.Name, Description: "produce acceptable " + core.TypeName[Out](), Preconditions: []string{acceptKey}})},
	}), nil
}
