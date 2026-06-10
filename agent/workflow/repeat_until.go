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

	// MaxIterations bounds the loop. <=0 defaults to 3, matching
	// embabel's RepeatUntil default. The workflow always runs Task
	// at least once.
	MaxIterations int

	// Task is the per-iteration body. It receives the loop input In,
	// the running history (so it can inspect prior attempts), and
	// returns the next attempt. Required.
	Task func(ctx context.Context, pc *core.ProcessContext, in In, history *History[Out]) (Out, error)

	// Accept inspects the latest attempt and returns true to stop
	// the loop. Receives the loop input, the latest Out, and the
	// full history (latest is also history.Last()). Required.
	Accept func(ctx context.Context, in In, last Out, history *History[Out]) bool
}

// RepeatUntil compiles spec into a deployable [*core.Agent].
//
// The agent has one action — "{Name}-task" — that produces Out and
// is flagged [ActionConfig.CanRerun] so the planner can pick it
// repeatedly. After every run the runtime re-evaluates the
// "{Name}-acceptable" computed condition: when true, the goal
// (which preconditions on it) is satisfied and the loop terminates;
// when false, GOAP re-plans and runs the task again. The
// MaxIterations cap forces the condition to true regardless once
// reached, so a never-accepting Accept can't loop forever.
//
// History is bound on the blackboard at first iteration and
// mutated (via Append) on each subsequent run, so user-supplied
// Task / Accept callbacks always see the running record.
//
// Returns an error on missing Name, nil Task, or nil Accept.
func RepeatUntil[In, Out any](spec RepeatUntilConfig[In, Out]) (*core.Agent, error) {
	if spec.Name == "" {
		return nil, errors.New("workflow.RepeatUntil: Name must not be empty")
	}
	if spec.Task == nil {
		return nil, errors.New("workflow.RepeatUntil: Task must not be nil")
	}
	if spec.Accept == nil {
		return nil, errors.New("workflow.RepeatUntil: Accept must not be nil")
	}
	maxIter := spec.MaxIterations
	if maxIter <= 0 {
		maxIter = 3
	}

	// Condition keys must not contain ':' — the determiner reserves
	// that for type-binding keys. Use '_' as the separator.
	acceptKey := spec.Name + "_acceptable"

	acceptCondition := core.NewCondition(acceptKey, func(ctx context.Context, env *core.ConditionEnv) core.Determination {
		history, ok := core.Last[*History[Out]](env.Blackboard)
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
		in, _ := core.Last[In](env.Blackboard)
		if spec.Accept(ctx, in, last, history) {
			return core.True
		}
		return core.False
	})

	task := core.NewAction[In, Out](
		spec.Name+"-task",
		func(ctx context.Context, pc *core.ProcessContext, in In) (Out, error) {
			history, ok := core.Last[*History[Out]](pc.Blackboard)
			if !ok {
				history = &History[Out]{}
				pc.Blackboard.Bind(history)
			}
			out, err := spec.Task(ctx, pc, in, history)
			if err != nil {
				var zero Out
				return zero, err
			}
			history.record(out)
			return out, nil
		},
		core.ActionConfig{
			Description: "loop body — produces a candidate Out",
			CanRerun:    true,
			Post:        []string{acceptKey},
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        spec.Name,
		Description: spec.Description,
		Actions:     []core.Action{task},
		Conditions:  []core.Condition{acceptCondition},
		Goals: []*core.Goal{core.GoalProducing[Out](core.Goal{
			Name:        spec.Name,
			Description: "produce acceptable " + core.TypeName[Out](),
			Pre:         []string{acceptKey},
		})},
	}), nil
}
