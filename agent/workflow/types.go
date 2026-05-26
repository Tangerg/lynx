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

// ResultList is the typed wrapper [ScatterGatherConfig] binds when its
// generators all complete, so the join action picks it up via a
// single-typed input binding.
type ResultList[Element any] struct {
	Items []Element
}
