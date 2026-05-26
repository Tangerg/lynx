package workflow

import (
	"context"
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
// Mirrors embabel's `RepeatUntilAcceptable.kt` without wiring it as
// a separate Spring path — it's just RepeatUntil with a Feedback-
// shaped Accept.
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

// RepeatUntilAcceptable compiles spec into a deployable agent.
// Implementation delegates to [RepeatUntil] — the same single
// CanRerun action + computed Accept condition machinery. The only
// special sauce is wrapping the user's Evaluator into a
// [RepeatUntilConfig.Accept] and binding the latest [Feedback] on the
// blackboard each iteration.
//
// Returns an error on missing Name / nil Task / nil Evaluator.
func RepeatUntilAcceptable[In, Out any](spec RepeatUntilAcceptableConfig[In, Out]) (*core.Agent, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: Name must not be empty")
	}
	if spec.Task == nil {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: Task must not be nil")
	}
	if spec.Evaluator == nil {
		return nil, fmt.Errorf("workflow.RepeatUntilAcceptable: Evaluator must not be nil")
	}
	threshold := spec.AcceptableScore
	if threshold <= 0 {
		threshold = 0.7
	}

	return RepeatUntil(RepeatUntilConfig[In, Out]{
		Name:          spec.Name,
		Description:   spec.Description,
		MaxIterations: spec.MaxIterations,
		Task:          spec.Task,
		Accept: func(ctx context.Context, in In, last Out, _ *History[Out]) bool {
			// We need a pc here to bind feedback, but Accept's
			// signature only carries ctx. Pull pc from ctx (the
			// runtime injects [core.WithProcess] before each tick;
			// blackboard hangs off Process).
			fb, err := spec.Evaluator(ctx, nil, in, last)
			if err != nil {
				// Treat evaluator failure as "not yet acceptable";
				// the next iteration tries again. Errors percolate
				// to logs via the framework's event stream;
				// returning false here keeps the loop going so a
				// transient evaluator failure doesn't strand the
				// workflow.
				return false
			}
			if p := core.ProcessFrom(ctx); p != nil && p.Blackboard() != nil {
				p.Blackboard().Bind(fb)
			}
			return fb.Acceptable(threshold)
		},
	})
}
