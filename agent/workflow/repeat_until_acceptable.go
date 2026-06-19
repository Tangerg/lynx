package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// Evaluator is the user-supplied "did this attempt meet the bar?"
// callback driving [RepeatUntilAcceptableConfig]. It receives the
// loop's input and the latest attempt; returns a [Feedback] whose
// Score gates the loop. Typical implementation: ask an LLM judge
// for a score 0..1 + rationale.
type Evaluator[In, Out any] func(ctx context.Context, pc *core.ProcessContext, in In, last Out) (Feedback, error)

// RepeatUntilAcceptableConfig is a thin shim over [RepeatUntilConfig]
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

	// MaxIterations bounds the loop. <=0 defaults to 3 (same as
	// [RepeatUntilConfig]).
	MaxIterations int

	// AcceptableScore is the [Feedback.Score] threshold; the loop
	// terminates as soon as Evaluator returns Score ≥ this. <=0
	// defaults to 0.7.
	AcceptableScore float64

	// Task produces a fresh attempt. Same shape as
	// [RepeatUntilConfig.Task] — receives loop input, current
	// history (so the body can "revise based on prior feedback"),
	// and returns the next Out.
	Task func(ctx context.Context, pc *core.ProcessContext, in In, history *History[Out]) (Out, error)

	// Evaluator scores the latest Out. The returned Feedback is
	// also bound on the blackboard (Bind) so subsequent Task calls
	// can fetch it via [core.Last][Feedback].
	Evaluator Evaluator[In, Out]
}

// RepeatUntilAcceptable compiles spec into a deployable agent. Unlike a
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
// A nil/erroring Evaluator result for one attempt is recorded as score 0
// (with the error in Feedback.Text) and the loop continues, so a transient
// evaluation failure can't strand the workflow.
//
// Returns an error on missing Name / nil Task / nil Evaluator.
func RepeatUntilAcceptable[In, Out any](spec RepeatUntilAcceptableConfig[In, Out]) (*core.Agent, error) {
	if spec.Name == "" {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Name must not be empty")
	}
	if spec.Task == nil {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Task must not be nil")
	}
	if spec.Evaluator == nil {
		return nil, errors.New("workflow.RepeatUntilAcceptable: Evaluator must not be nil")
	}
	threshold := spec.AcceptableScore
	if threshold <= 0 {
		threshold = 0.7
	}
	maxIter := spec.MaxIterations
	if maxIter <= 0 {
		maxIter = 3
	}

	acceptKey := spec.Name + "_acceptable"

	acceptCondition := core.NewCondition(acceptKey, func(_ context.Context, env *core.ConditionEnv) core.Determination {
		history, ok := core.Last[*AttemptHistory[Out]](env.Blackboard)
		if !ok {
			return core.False
		}
		if history.Count() >= maxIter {
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
		spec.Name+"-task",
		func(ctx context.Context, pc *core.ProcessContext, in In) (Out, error) {
			var zero Out

			history, ok := core.Last[*AttemptHistory[Out]](pc.Blackboard)
			if !ok {
				history = &AttemptHistory[Out]{}
				pc.Blackboard.Bind(history)
			}

			// The task sees prior outputs so it can revise.
			out, err := spec.Task(ctx, pc, in, &History[Out]{Attempts: history.outputs()})
			if err != nil {
				return zero, err
			}

			fb, evalErr := spec.Evaluator(ctx, pc, in, out)
			if evalErr != nil {
				// Keep the attempt (score 0) and keep looping rather than
				// failing the whole workflow on a transient eval error.
				fb = Feedback{Score: 0, Text: fmt.Sprintf("evaluation failed: %v", evalErr)}
			}
			history.record(out, fb)
			pc.Blackboard.Bind(fb)

			best, _ := history.Best()
			return best.Output, nil
		},
		core.ActionConfig{
			Description: "evaluator-optimizer loop body — produces, scores, keeps the best",
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
			Description: "produce best-scoring " + core.TypeName[Out](),
			Pre:         []string{acceptKey},
		})},
	}), nil
}
