package workflow

import (
	"github.com/Tangerg/lynx/agent/core"
)

// singleAttempt is the QoS shared by every workflow-internal action.
// Workflow orchestration actions (scatter / gather / loop body) treat
// their failure as a deterministic outcome the caller already handles
// — retries with exponential back-off would just stretch a guaranteed
// failure across minutes. Domain actions wrapped *inside* a workflow
// (the user's generators / task body) keep their own retry policies.
var singleAttempt = core.ActionQoS{MaxAttempts: 1}

// Feedback is a numeric + textual acceptance signal — the canonical
// shape for "did this output meet the bar?". Score is in [0, 1] (0
// worst, 1 best); Text gives a human-readable explanation. Use as
// the return value of an [RepeatUntilConfig.Accept] callback that wants
// to attach an LLM-supplied verdict, or as a regular blackboard
// artifact.
type Feedback struct {
	Score float64
	Text  string
}

// Acceptable reports Score ≥ threshold. Convenience for the common
// "accept when score ≥ 0.7" check inside an [RepeatUntilConfig.Accept].
func (f Feedback) Acceptable(threshold float64) bool { return f.Score >= threshold }

// loopInput tags a loop's ORIGINAL input on the blackboard so RepeatUntil /
// Loop can recover it distinctly from the per-iteration outputs. When In and
// Out are the SAME Go type — the canonical refinement loop ("improve this Draft
// until it's good enough") — every iteration's Out binding shadows the input,
// so the framework's typed In binding (and core.Last[In]) would return the
// latest ATTEMPT, not the original input. loopInput[T] is a distinct type from
// T, so looking it up never picks up an attempt. Bound once on the first
// iteration; the task + the accept/until condition read the input back through
// it. See [RepeatUntil] / [Loop].
type loopInput[T any] struct{ value T }

// History tracks every attempt produced by a [RepeatUntilConfig.Task]
// in the order they ran. The Accept callback receives a *History so
// it can inspect prior attempts (e.g., compute "is the score still
// improving?").
type History[T any] struct {
	// Attempts is the ordered list of every Out the task has
	// produced. The last element is the most recent attempt.
	Attempts []T
}

// Last returns the most recent attempt and true, or the zero value
// and false when no attempt has been recorded.
func (h *History[T]) Last() (T, bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero T
		return zero, false
	}
	return h.Attempts[len(h.Attempts)-1], true
}

// Count is shorthand for len(h.Attempts), nil-safe.
func (h *History[T]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.Attempts)
}

// record appends a new attempt; private — only the workflow builder
// pushes into history.
func (h *History[T]) record(attempt T) {
	if h == nil {
		return
	}
	h.Attempts = append(h.Attempts, attempt)
}

// Attempt pairs one task output with the [Feedback] it earned. It is the
// element of an [AttemptHistory], so [RepeatUntilAcceptable] can return the
// highest-scoring attempt rather than merely the last one.
type Attempt[Out any] struct {
	Output   Out
	Feedback Feedback
}

// AttemptHistory records every (output, feedback) pair an evaluator-driven
// loop produced, in order. It backs [RepeatUntilAcceptable]'s best-of-N
// behavior and is bound on the process blackboard so callers can inspect
// the full record (e.g. core.Last[*AttemptHistory[Out]]).
type AttemptHistory[Out any] struct {
	Attempts []Attempt[Out]
}

// record appends an (output, feedback) pair; private — only the workflow
// builder pushes into it.
func (h *AttemptHistory[Out]) record(out Out, fb Feedback) {
	if h == nil {
		return
	}
	h.Attempts = append(h.Attempts, Attempt[Out]{Output: out, Feedback: fb})
}

// Count is the number of attempts recorded, nil-safe.
func (h *AttemptHistory[Out]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.Attempts)
}

// Last returns the most recent attempt, or false when none recorded.
func (h *AttemptHistory[Out]) Last() (Attempt[Out], bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	return h.Attempts[len(h.Attempts)-1], true
}

// Best returns the attempt with the highest [Feedback.Score], or false when
// none recorded. Ties resolve to the earliest attempt (stable).
func (h *AttemptHistory[Out]) Best() (Attempt[Out], bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	best := h.Attempts[0]
	for _, a := range h.Attempts[1:] {
		if a.Feedback.Score > best.Feedback.Score {
			best = a
		}
	}
	return best, true
}

// outputs returns just the produced outputs, in order — used to present a
// [History] view to the task callback for revision.
func (h *AttemptHistory[Out]) outputs() []Out {
	if h == nil {
		return nil
	}
	out := make([]Out, len(h.Attempts))
	for i, a := range h.Attempts {
		out[i] = a.Output
	}
	return out
}

// ResultList is the typed wrapper [ScatterGatherConfig] binds when its
// generators all complete, so the join action picks it up via a
// single-typed input binding.
type ResultList[Element any] struct {
	Items []Element
}
