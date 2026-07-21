package workflow

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/agent/core"
)

// DefaultAcceptableScore is the acceptance threshold used when none is set.
const DefaultAcceptableScore = 0.7

// Evaluator is the user-supplied "did this attempt meet the bar?"
// callback driving [RepeatUntilAcceptableConfig]. It receives the
// loop's input and the latest attempt; returns a [Feedback] whose
// Score gates the loop. Typical implementation: ask an LLM judge
// for a score 0..1 + rationale.
type Evaluator[In, Out any] func(ctx context.Context, process *core.ProcessContext, input In, latest Out) (Feedback, error)

// RepeatUntilAcceptableConfig is a specialized wrapper over [RepeatUntilConfig]
// that turns the "loop until LLM is satisfied" pattern into a
// configuration: supply Task + Evaluator + AcceptableScore, and
// the workflow loops until the evaluator's Score crosses the
// threshold (or [MaxIterations] expires).
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

	// AcceptableScore is the [Feedback.Score] threshold; the loop
	// terminates as soon as Evaluator returns Score ≥ this. Zero defaults to
	// 0.7; negative values are invalid.
	AcceptableScore float64

	// Task produces a fresh attempt. Same shape as
	// [RepeatUntilConfig.Task] — receives loop input, current
	// history (so the body can "revise based on prior feedback"),
	// and returns the next Out.
	Task func(ctx context.Context, process *core.ProcessContext, input In, history *History[Out]) (Out, error)

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
// Returns an error on missing Name / nil Task / nil Evaluator.
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
	threshold := config.AcceptableScore
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: AcceptableScore %v must be finite and between 0 and 1", threshold)
	}
	if threshold == 0 {
		threshold = DefaultAcceptableScore
	}
	if config.MaxIterations < 0 {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: MaxIterations %d must not be negative", config.MaxIterations)
	}
	maxIterations := config.MaxIterations
	if maxIterations == 0 {
		maxIterations = DefaultRepeatIterations
	}

	acceptKey := config.Name + "_acceptable"
	historyState := core.NewBinding[*AttemptHistory[Out]](config.Name + historyStateSuffix)
	inputState := core.NewBinding[loopInput[In]](config.Name + inputStateSuffix)
	feedbackState := core.NewBinding[Feedback](config.Name + feedbackStateSuffix)

	acceptCondition := core.NewCondition(acceptKey, func(_ context.Context, env *core.ConditionEnv) core.Truth {
		history, ok := core.Last[*AttemptHistory[Out]](env.Blackboard)
		if !ok {
			return core.False
		}
		if history.Count() >= maxIterations {
			return core.True
		}
		best, ok := history.Best()
		if !ok {
			return core.False
		}
		if best.Feedback.Acceptable(threshold) {
			return core.True
		}
		return core.False
	})

	task := core.NewAction[In, Out](
		config.Name+"-task",
		func(ctx context.Context, process *core.ProcessContext, input In) (Out, error) {
			var zero Out

			history, ok := core.Last[*AttemptHistory[Out]](process.Blackboard())
			if !ok {
				history = &AttemptHistory[Out]{}
				process.Blackboard().Store(historyState.Name, history)
				process.Blackboard().Store(inputState.Name, loopInput[In]{Value: input})
			} else if original, found := core.Last[loopInput[In]](process.Blackboard()); found {
				input = original.Value
			}

			// The task sees prior outputs so it can revise.
			output, err := config.Task(ctx, process, input, newHistory(history.outputs()))
			if err != nil {
				return zero, err
			}

			feedback, err := config.Evaluator(ctx, process, input, output)
			if err != nil {
				return zero, fmt.Errorf("workflow.RepeatUntilAcceptable: evaluate attempt: %w", err)
			}
			if err := feedback.Validate(); err != nil {
				return zero, fmt.Errorf("workflow.RepeatUntilAcceptable: evaluator feedback: %w", err)
			}
			history.record(output, feedback)
			process.Blackboard().Store(feedbackState.Name, feedback)

			best, _ := history.Best()
			return best.Output, nil
		},
		core.ActionConfig{
			Description: "evaluator-optimizer loop body — produces, scores, keeps the best",
			Repeatable:  true,
			Effects:     []string{acceptKey},
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:         config.Name,
		Description:  config.Description,
		Actions:      []core.Action{task},
		Conditions:   []core.Condition{acceptCondition},
		DurableState: []core.Binding{historyState, inputState, feedbackState},
		Goals:        []*core.Goal{core.NewOutputGoal[Out](core.GoalConfig{Name: config.Name, Description: "produce best-scoring " + core.TypeName[Out](), Preconditions: []string{acceptKey}})},
	}), nil
}
