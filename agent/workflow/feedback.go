package workflow

// Feedback is a scored, human-readable acceptance signal. Score is in [0, 1],
// where 0 is worst and 1 is best.
type Feedback struct {
	Score float64
	Text  string
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
	Attempts []Attempt[Out]
}

func (h *AttemptHistory[Out]) record(output Out, feedback Feedback) {
	if h == nil {
		return
	}
	h.Attempts = append(h.Attempts, Attempt[Out]{Output: output, Feedback: feedback})
}

// Count reports the number of recorded attempts. It is nil-safe.
func (h *AttemptHistory[Out]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.Attempts)
}

// Last returns the most recent attempt.
func (h *AttemptHistory[Out]) Last() (Attempt[Out], bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	return h.Attempts[len(h.Attempts)-1], true
}

// Best returns the highest-scoring attempt. Ties keep the earliest attempt.
func (h *AttemptHistory[Out]) Best() (Attempt[Out], bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero Attempt[Out]
		return zero, false
	}
	best := h.Attempts[0]
	for _, attempt := range h.Attempts[1:] {
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
	outputs := make([]Out, len(h.Attempts))
	for index, attempt := range h.Attempts {
		outputs[index] = attempt.Output
	}
	return outputs
}
