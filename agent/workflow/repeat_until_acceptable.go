package workflow

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/agent/core"
)

// Evaluator is the user-supplied "did this attempt meet the bar?"
// callback driving [RepeatUntilAcceptableConfig]. It receives the
// loop's input and the latest attempt; returns a [Feedback] whose
// Score gates the loop. Typical implementation: ask an LLM judge
// for a score 0..1 + rationale.
type Evaluator[In, Out any] func(ctx context.Context, process *core.ProcessContext, input In, latest Out) (Feedback, error)

// RepeatUntilAcceptableConfig is a specialized wrapper over [RepeatUntilConfig]
// that turns the "loop until LLM is satisfied" pattern into a
// configuration: supply Task + Evaluator + AcceptableScore (required), and
// the workflow loops until the evaluator's Score crosses the
// threshold (or [RepeatUntilAcceptableConfig.MaxIterations] expires).
//
// Each iteration's Feedback is also bound on the blackboard via
// [core.Blackboard.Bind] so users can inspect "why did the judge
// reject the previous attempt" via blackboard tools — useful when
// Task wants to read prior feedback to revise.
//
// RepeatUntilAcceptable: RepeatUntil with a Feedback-shaped Accept.
type RepeatUntilAcceptableConfig[In, Out any] struct {
	// Name names the produced agent. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxIterations bounds the loop. Zero defaults to 3 (same as
	// [RepeatUntilConfig]); negative values are invalid.
	MaxIterations int

	// AcceptableScore is the required [Feedback.Score] threshold; the loop
	// terminates as soon as Evaluator returns Score ≥ this. It must be finite
	// and greater than zero through one, inclusive.
	AcceptableScore float64

	// Task produces a fresh attempt. Same shape as
	// [RepeatUntilConfig.Task] — receives loop input, current
	// history (so the body can "revise based on prior feedback"),
	// and returns the next Out.
	Task func(ctx context.Context, process *core.ProcessContext, input In, history History[Out]) (Out, error)

	// Evaluator scores the latest Out. The returned Feedback is
	// also bound on the blackboard (Bind) so subsequent Task calls
	// can fetch it via [core.Last][Feedback].
	Evaluator Evaluator[In, Out]
}

// RepeatUntilAcceptable compiles config into a deployable agent. Unlike a
// plain [RepeatUntil], it evaluates each attempt inside the task action,
// records every (output, feedback) pair in an [AttemptHistory], and
// produces the highest-scoring attempt rather than merely the last
// accepted one — so a later, worse attempt never overwrites an earlier,
// better one (best-of-N semantics).
//
// Per iteration the action: runs Task (which sees prior outputs for
// revision), scores it via Evaluator, records the pair, binds the latest
// Feedback (for introspection) and the running AttemptHistory, and returns
// the best attempt so far. The "{Name}_acceptable" condition stops the loop
// once the best score crosses the threshold or MaxIterations is reached.
//
// Returns an error on missing Name, nil Task, nil Evaluator, a non-positive or
// non-finite AcceptableScore, a score above one, or negative MaxIterations.
func RepeatUntilAcceptable[In, Out any](config RepeatUntilAcceptableConfig[In, Out]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Name must not be empty")
	}
	if config.Task == nil {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Task must not be nil")
	}
	if config.Evaluator == nil {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Evaluator must not be nil")
	}
	// What counts as good enough is the author's judgement, not something this
	// package can guess, so an unset threshold is an error rather than a number
	// picked here.
	threshold := config.AcceptableScore
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold <= 0 || threshold > 1 {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: AcceptableScore %v must be set and between 0 (exclusive) and 1", threshold)
	}
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: MaxIterations %d must not be negative", config.MaxIterations)
	}
	maxIterations := cmp.Or(config.MaxIterations, defaultRepeatIterations)

	acceptKey := config.Name + "_acceptable"
	historyState := core.NewBinding[AttemptHistory[Out]](config.Name + historyStateSuffix)
	feedbackState := core.NewBinding[Feedback](config.Name + feedbackStateSuffix)

	return compileRepeatWorkflow(repeatWorkflowConfig[In, Out, AttemptHistory[Out]]{
		name:          config.Name,
		description:   config.Description,
		actionName:    config.Name + "-task",
		doneKey:       acceptKey,
		maxIterations: maxIterations,
		stateBinding:  historyState,
		newState:      func() AttemptHistory[Out] { return AttemptHistory[Out]{} },
		count:         AttemptHistory[Out].Count,
		snapshotState: []core.Binding{feedbackState},
		run: func(ctx context.Context, process *core.ProcessContext, input In, history AttemptHistory[Out]) (Out, AttemptHistory[Out], error) {
			var zero Out

			output, err := config.Task(ctx, process, input, newHistory(history.outputs()))
			if err != nil {
				return zero, history, err
			}

			feedback, err := config.Evaluator(ctx, process, input, output)
			if err != nil {
				return zero, history, fmt.Errorf("workflow.RepeatUntilAcceptable: evaluate attempt: %w", err)
			}
			if err := feedback.Validate(); err != nil {
				return zero, history, fmt.Errorf("workflow.RepeatUntilAcceptable: evaluator feedback: %w", err)
			}
			history = history.withAttempt(output, feedback)
			if err := process.Blackboard().Store(feedbackState.Name, feedback); err != nil {
				return zero, history, fmt.Errorf("workflow.RepeatUntilAcceptable: store feedback: %w", err)
			}

			best, _ := history.Best()
			return best.Output, history, nil
		},
		stop: func(_ context.Context, _ In, history AttemptHistory[Out]) bool {
			best, ok := history.Best()
			return ok && best.Feedback.Acceptable(threshold)
		},
	}), nil
}
