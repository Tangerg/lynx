package execution

import (
	"fmt"
	"time"
)

// FailureKind classifies an execution failure without depending on provider
// error text. Adapters translate concrete transport and SDK errors at their
// boundary; the application projects this stable vocabulary to its transcript
// and wire problem types.
type FailureKind uint8

const (
	FailureInternal FailureKind = iota
	FailureAgentStuck
	FailureRateLimited
	FailureInvalidCredentials
	FailureTimeout
	FailureProviderUnavailable
	FailureProviderRejected
)

// Failure carries a typed execution classification while preserving the
// original error chain for diagnostics. RetryAfter is meaningful only for
// retryable kinds and may be zero when the provider supplied no hint.
type Failure struct {
	Kind       FailureKind
	RetryAfter time.Duration
	Err        error
}

func (e *Failure) Error() string {
	if e == nil {
		return "execution failure"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("execution failure kind %d", e.Kind)
}

func (e *Failure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
