package workflow

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

const (
	historyStateSuffix  = "_history"
	inputStateSuffix    = "_input"
	feedbackStateSuffix = "_feedback"
)

// loopInput keeps a loop's original input distinct from later outputs of the
// same Go type on the blackboard.
type loopInput[T any] struct {
	Value T `json:"value"`
}

// History records task outputs in execution order.
type History[T any] struct {
	attempts []T
}

func newHistory[T any](attempts []T) *History[T] {
	return &History[T]{attempts: slices.Clone(attempts)}
}

// Attempts returns an ownership-isolated snapshot in execution order.
func (h *History[T]) Attempts() []T {
	if h == nil {
		return nil
	}
	return slices.Clone(h.attempts)
}

// MarshalJSON persists the private attempt sequence without exposing mutable
// framework state through the Go API.
func (h History[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Attempts []T `json:"attempts"`
	}{Attempts: h.attempts})
}

// UnmarshalJSON restores a history while retaining ownership of its sequence.
func (h *History[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		Attempts []T `json:"attempts"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	h.attempts = slices.Clone(wire.Attempts)
	return nil
}

// Last returns the most recent attempt.
func (h *History[T]) Last() (T, bool) {
	if h == nil || len(h.attempts) == 0 {
		var zero T
		return zero, false
	}
	return h.attempts[len(h.attempts)-1], true
}

// Count reports the number of recorded attempts. It is nil-safe.
func (h *History[T]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.attempts)
}

func (h *History[T]) record(attempt T) {
	if h == nil {
		return
	}
	h.attempts = append(h.attempts, attempt)
}

// Feedback is a scored, human-readable acceptance signal. Score is in [0, 1].
type Feedback struct {
	Score float64
	Text  string
}

// Validate verifies the normalized score contract.
func (f Feedback) Validate() error {
	if math.IsNaN(f.Score) || math.IsInf(f.Score, 0) || f.Score < 0 || f.Score > 1 {
		return fmt.Errorf("workflow.Feedback: score %v must be finite and within [0, 1]", f.Score)
	}
	return nil
}

// Acceptable reports whether Score meets threshold.
func (f Feedback) Acceptable(threshold float64) bool { return f.Score >= threshold }

// Attempt pairs one task output with its feedback.
type Attempt[Out any] struct {
	Output   Out
	Feedback Feedback
}

// AttemptHistory records evaluator-driven attempts in execution order.
type AttemptHistory[Out any] struct {
	attempts []Attempt[Out]
}

// Attempts returns an ownership-isolated snapshot in execution order.
func (h *AttemptHistory[Out]) Attempts() []Attempt[Out] {
	if h == nil {
		return nil
	}
	return slices.Clone(h.attempts)
}

// MarshalJSON persists the private attempt sequence without exposing mutable
// framework state through the Go API.
func (h AttemptHistory[Out]) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Attempts []Attempt[Out] `json:"attempts"`
	}{Attempts: h.attempts})
}

// UnmarshalJSON restores a history while retaining ownership of its sequence.
func (h *AttemptHistory[Out]) UnmarshalJSON(data []byte) error {
	var wire struct {
		Attempts []Attempt[Out] `json:"attempts"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	h.attempts = slices.Clone(wire.Attempts)
	return nil
}

func (h *AttemptHistory[Out]) record(output Out, feedback Feedback) {
	if h == nil {
		return
	}
	h.attempts = append(h.attempts, Attempt[Out]{Output: output, Feedback: feedback})
}

// Count reports the number of recorded attempts. It is nil-safe.
func (h *AttemptHistory[Out]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.attempts)
}

// Last returns the most recent attempt.
func (h *AttemptHistory[Out]) Last() (Attempt[Out], bool) {
	if h == nil || len(h.attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	return h.attempts[len(h.attempts)-1], true
}

// Best returns the highest-scoring attempt. Ties keep the earliest attempt.
func (h *AttemptHistory[Out]) Best() (Attempt[Out], bool) {
	if h == nil || len(h.attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	best := h.attempts[0]
	for _, attempt := range h.attempts[1:] {
		if attempt.Feedback.Score > best.Feedback.Score {
			best = attempt
		}
	}
	return best, true
}

func (h *AttemptHistory[Out]) outputs() []Out {
	if h == nil {
		return nil
	}
	outputs := make([]Out, len(h.attempts))
	for index, attempt := range h.attempts {
		outputs[index] = attempt.Output
	}
	return outputs
}
