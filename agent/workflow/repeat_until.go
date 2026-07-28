package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// DefaultRepeatIterations bounds repeat workflows when MaxIterations is unset.
// The bound exists for the same reason as [DefaultLoopIterations]: iterations
// accumulate framework-owned state, so the loop must terminate even when the
// author states no ceiling.
const DefaultRepeatIterations = 3

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

	// MaxIterations bounds the loop. Zero defaults to 3; negative values are
	// invalid. The workflow always runs Task at least once.
	MaxIterations int

	// Task is the per-iteration body. It receives the loop input In,
	// the running history (so it can inspect prior attempts), and
	// returns the next attempt. Required.
	Task func(ctx context.Context, process *core.ProcessContext, input In, history History[Out]) (Out, error)

	// Accept inspects the latest attempt and returns true to stop
	// the loop. Receives the loop input, the latest Out, and the
	// full history (latest is also history.Last()). Required.
	Accept func(ctx context.Context, input In, latest Out, history History[Out]) bool
}

// RepeatUntil compiles config into a deployable [*core.Agent].
//
// The agent has one action — "{Name}-task" — that produces Out and
// is flagged [core.ActionConfig.Repeatable] so the planner can pick it
// repeatedly. After every run the runtime re-evaluates the
// "{Name}-acceptable" computed condition: when true, the goal
// (which preconditions on it) is satisfied and the loop terminates;
// when false, GOAP re-plans and runs the task again. The
// MaxIterations cap forces the condition to true regardless once
// reached, so a never-accepting Accept can't loop forever.
//
// History is replaced on the blackboard after each iteration, so user-supplied
// Task / Accept callbacks see an immutable snapshot of the running record.
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
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("workflow.RepeatUntil: MaxIterations %d must not be negative", config.MaxIterations)
	}
	maxIterations := config.MaxIterations
	if maxIterations == 0 {
		maxIterations = DefaultRepeatIterations
	}

	acceptKey := config.Name + "_acceptable"
	historyState := core.NewBinding[History[Out]](config.Name + historyStateSuffix)

	return compileRepeatWorkflow(repeatWorkflowConfig[In, Out, History[Out]]{
		name:          config.Name,
		description:   config.Description,
		actionName:    config.Name + "-task",
		doneKey:       acceptKey,
		maxIterations: maxIterations,
		stateBinding:  historyState,
		newState:      func() History[Out] { return History[Out]{} },
		count:         History[Out].Count,
		run: func(ctx context.Context, process *core.ProcessContext, input In, history History[Out]) (Out, History[Out], error) {
			output, err := config.Task(ctx, process, input, history)
			if err != nil {
				var zero Out
				return zero, history, err
			}
			return output, history.withAttempt(output), nil
		},
		stop: func(ctx context.Context, input In, history History[Out]) bool {
			last, ok := history.Last()
			return ok && config.Accept(ctx, input, last, history)
		},
	}), nil
}
