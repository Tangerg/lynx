// Package modelcall owns the product meaning of a failed model invocation.
package modelcall

import "fmt"

// FailureKind is the closed provider-failure vocabulary understood by Runs.
type FailureKind string

const (
	FailureRateLimited        FailureKind = "rate_limited"
	FailureInvalidCredentials FailureKind = "invalid_credentials"
	FailureTimeout            FailureKind = "timeout"
	FailureUnavailable        FailureKind = "provider_unavailable"
	FailureRejected           FailureKind = "provider_rejected"
)

// Failure is a secret-free model failure suitable for crossing process and
// framework boundaries. Provider response bodies deliberately stay outside it.
type Failure struct {
	kind              FailureKind
	retryAfterSeconds int
}

func NewFailure(kind FailureKind, retryAfterSeconds int) (Failure, error) {
	if !kind.Valid() {
		return Failure{}, fmt.Errorf("modelcall: invalid failure kind %q", kind)
	}
	if retryAfterSeconds < 0 {
		return Failure{}, fmt.Errorf("modelcall: retry-after cannot be negative")
	}
	if kind != FailureRateLimited && kind != FailureUnavailable && retryAfterSeconds != 0 {
		return Failure{}, fmt.Errorf("modelcall: %s cannot carry retry-after", kind)
	}
	return Failure{kind: kind, retryAfterSeconds: retryAfterSeconds}, nil
}

func (kind FailureKind) Valid() bool {
	switch kind {
	case FailureRateLimited,
		FailureInvalidCredentials,
		FailureTimeout,
		FailureUnavailable,
		FailureRejected:
		return true
	default:
		return false
	}
}

func (failure Failure) Valid() bool {
	_, err := NewFailure(failure.kind, failure.retryAfterSeconds)
	return err == nil
}

func (failure Failure) Kind() FailureKind { return failure.kind }

func (failure Failure) RetryAfterSeconds() int { return failure.retryAfterSeconds }

func (failure Failure) Detail() string {
	switch failure.kind {
	case FailureRateLimited:
		return "the model provider rate limited the request"
	case FailureInvalidCredentials:
		return "the model provider rejected the configured credential"
	case FailureTimeout:
		return "the model provider request timed out"
	case FailureUnavailable:
		return "the model provider is temporarily unavailable"
	case FailureRejected:
		return "the model provider rejected the request"
	default:
		return "the model request failed"
	}
}
