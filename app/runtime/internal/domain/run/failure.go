package run

import (
	"fmt"
	"time"
)

// FailureKind classifies a Run failure without depending on provider
// error text. Integrations translate concrete failures at their boundary;
// durable Run records retain this stable vocabulary.
type FailureKind uint8

const (
	FailureInternal FailureKind = iota
	FailureLost
	FailureAgentStuck
	FailureRateLimited
	FailureInvalidCredentials
	FailureTimeout
	FailureProviderUnavailable
	FailureProviderRejected
)

// String names the kind for diagnostics — parity with the package's other
// enums (State / Outcome), so a FailureError without an error chain reports a
// legible name instead of a raw integer.
func (k FailureKind) String() string {
	switch k {
	case FailureInternal:
		return "internal"
	case FailureLost:
		return "lost"
	case FailureAgentStuck:
		return "agent_stuck"
	case FailureRateLimited:
		return "rate_limited"
	case FailureInvalidCredentials:
		return "invalid_credentials"
	case FailureTimeout:
		return "timeout"
	case FailureProviderUnavailable:
		return "provider_unavailable"
	case FailureProviderRejected:
		return "provider_rejected"
	default:
		return fmt.Sprintf("kind(%d)", uint8(k))
	}
}

// Failure is the durable, provider-neutral explanation for a failed Run.
type Failure struct {
	Kind       FailureKind
	Detail     string
	DocURL     string
	RetryAfter time.Duration
}

// Validate reports whether the failure carries a known kind and a meaningful
// retry delay only for retryable classifications.
func (failure Failure) Validate() error {
	if failure.Kind > FailureProviderRejected {
		return fmt.Errorf("run: unknown failure kind %d", failure.Kind)
	}
	if failure.RetryAfter < 0 {
		return fmt.Errorf("run: failure retry delay must not be negative")
	}
	if failure.RetryAfter > 0 {
		switch failure.Kind {
		case FailureRateLimited, FailureTimeout, FailureProviderUnavailable:
		default:
			return fmt.Errorf("run: failure kind %s cannot carry a retry delay", failure.Kind)
		}
	}
	return nil
}

// FailureError carries a typed Run classification while preserving the
// original error chain for diagnostics. RetryAfter is meaningful only for
// retryable kinds and may be zero when the provider supplied no hint.
type FailureError struct {
	Kind       FailureKind
	RetryAfter time.Duration
	Err        error
}

func (e *FailureError) Error() string {
	if e == nil {
		return "run failure"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "run failure: " + e.Kind.String()
}

func (e *FailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
