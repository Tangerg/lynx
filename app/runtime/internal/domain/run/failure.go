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

// Valid reports whether f is part of the durable provider-neutral taxonomy.
func (f FailureKind) Valid() bool {
	switch f {
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
func (f FailureKind) String() string {
	if !f.Valid() {
		return "unknown"
	}
	return string(f)
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
func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("run: unknown failure kind %q", f.Kind)
	}
	if f.RetryAfter < 0 {
		return fmt.Errorf("run: failure retry delay must not be negative")
	}
	if f.RetryAfter > 0 {
		switch f.Kind {
		case FailureRateLimited, FailureTimeout, FailureProviderUnavailable:
		default:
			return fmt.Errorf("run: failure kind %s cannot carry a retry delay", f.Kind)
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

func (f *FailureError) Error() string {
	if f == nil {
		return "run failure"
	}
	if f.Err != nil {
		return f.Err.Error()
	}
	return "run failure: " + f.Kind.String()
}

func (f *FailureError) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Err
}
