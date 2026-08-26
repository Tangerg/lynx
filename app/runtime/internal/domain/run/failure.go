package run

import (
	"fmt"
	"time"
)

// FailureKind classifies a Run failure without depending on provider
// error text. Integrations translate concrete failures at their boundary;
// durable Run records retain this stable vocabulary.
type FailureKind string

const (
	FailureInternal            FailureKind = "internal"
	FailureLost                FailureKind = "lost"
	FailureAgentStuck          FailureKind = "agent_stuck"
	FailureRateLimited         FailureKind = "rate_limited"
	FailureInvalidCredentials  FailureKind = "invalid_credentials"
	FailureTimeout             FailureKind = "timeout"
	FailureProviderUnavailable FailureKind = "provider_unavailable"
	FailureProviderRejected    FailureKind = "provider_rejected"
)

// Valid reports whether k is part of the durable provider-neutral taxonomy.
func (k FailureKind) Valid() bool {
	switch k {
	case FailureInternal, FailureLost, FailureAgentStuck, FailureRateLimited,
		FailureInvalidCredentials, FailureTimeout, FailureProviderUnavailable,
		FailureProviderRejected:
		return true
	default:
		return false
	}
}

// String names the kind for diagnostics — parity with the package's other
// enums (State / Outcome), so a FailureError without an error chain reports a
// legible name instead of a raw integer.
func (k FailureKind) String() string {
	if !k.Valid() {
		return "unknown"
	}
	return string(k)
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
	if !failure.Kind.Valid() {
		return fmt.Errorf("run: unknown failure kind %q", failure.Kind)
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
