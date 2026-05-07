package core

import "time"

// ActionQoS governs retry behavior for a single action. Retry math itself
// (exponential backoff, jitter, overflow protection) is delegated to
// [github.com/Tangerg/lynx/pkg/retry]; this struct is just the policy
// surface the runtime translates into [retry.Option] values.
//
// Defaults are taken from embabel — aggressive (5 attempts) because LLM
// calls fail transiently more often than typical RPC.
type ActionQoS struct {
	// MaxAttempts caps total tries (initial + retries). 0 falls back to
	// the package default; the runtime treats anything < 1 as 1.
	MaxAttempts int

	// BaseDelay is the initial wait between attempts. Successive
	// attempts grow this exponentially (×2 per step) up to MaxDelay,
	// with random jitter added on each attempt.
	BaseDelay time.Duration

	// MaxDelay caps the per-attempt wait. 0 means uncapped.
	MaxDelay time.Duration
}

// DefaultActionQoS returns sensible production defaults: 5 attempts, 10s
// initial backoff, 60s cap.
func DefaultActionQoS() ActionQoS {
	return ActionQoS{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Second,
		MaxDelay:    60 * time.Second,
	}
}
