package core

import "time"

// ActionQos governs retry behavior for a single action. The defaults are
// taken from embabel; they're aggressive (5 attempts with exponential back-off)
// because LLM calls fail transiently more often than typical RPC.
type ActionQos struct {
	MaxAttempts       int
	BackoffMillis     int64
	BackoffMultiplier float64
	BackoffMaxMillis  int64
	Idempotent        bool
}

// DefaultActionQos returns sensible production defaults: 5 attempts, 10s
// initial back-off, 5× multiplier, 60s cap.
func DefaultActionQos() ActionQos {
	return ActionQos{
		MaxAttempts:       5,
		BackoffMillis:     10_000,
		BackoffMultiplier: 5.0,
		BackoffMaxMillis:  60_000,
		Idempotent:        false,
	}
}

// ShouldRetry decides whether the runtime should re-execute after a non-success
// status. ActionFailed is retryable; Waiting/Paused are intentional pauses and
// MUST NOT be retried.
func (q ActionQos) ShouldRetry(status ActionStatus) bool {
	return status == ActionFailed
}

// Backoff computes the wait between attempt N and attempt N+1, with cap.
// attempt is zero-indexed: Backoff(0) is the wait after the first failure.
func (q ActionQos) Backoff(attempt int) time.Duration {
	if q.BackoffMillis <= 0 {
		return 0
	}
	delay := float64(q.BackoffMillis)
	mult := q.BackoffMultiplier
	if mult <= 0 {
		mult = 1
	}
	for i := 0; i < attempt; i++ {
		delay *= mult
		if q.BackoffMaxMillis > 0 && delay > float64(q.BackoffMaxMillis) {
			delay = float64(q.BackoffMaxMillis)
			break
		}
	}
	return time.Duration(delay) * time.Millisecond
}
