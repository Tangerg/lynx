package workflow

// loopInput keeps a loop's original input distinct from later outputs of the
// same Go type on the blackboard.
type loopInput[T any] struct{ value T }

// History records task outputs in execution order.
type History[T any] struct {
	Attempts []T
}

// Last returns the most recent attempt.
func (h *History[T]) Last() (T, bool) {
	if h == nil || len(h.Attempts) == 0 {
		var zero T
		return zero, false
	}
	return h.Attempts[len(h.Attempts)-1], true
}

// Count reports the number of recorded attempts. It is nil-safe.
func (h *History[T]) Count() int {
	if h == nil {
		return 0
	}
	return len(h.Attempts)
}

func (h *History[T]) record(attempt T) {
	if h == nil {
		return
	}
	h.Attempts = append(h.Attempts, attempt)
}
