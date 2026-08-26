package tool

import (
	"errors"
	"fmt"
	"time"
)

// FailureKind classifies why one Tool invocation did not produce a successful
// result. It is deliberately separate from Run failure taxonomy.
type FailureKind string

const (
	FailureInternal         FailureKind = "internal"
	FailureDenied           FailureKind = "denied_by_user"
	FailureExecution        FailureKind = "tool_failed"
	FailureChildRunCanceled FailureKind = "child_run_canceled"
	FailureCanceled         FailureKind = "tool_canceled"
)

// Valid reports whether f belongs to the durable Tool failure taxonomy.
func (f FailureKind) Valid() bool {
	switch f {
	case FailureInternal, FailureDenied, FailureExecution, FailureChildRunCanceled, FailureCanceled:
		return true
	default:
		return false
	}
}

// String returns the stable durable name of f.
func (f FailureKind) String() string {
	if !f.Valid() {
		return "unknown"
	}
	return string(f)
}

// Failure is the durable explanation attached to an incomplete ToolCall.
type Failure struct {
	Kind       FailureKind
	Detail     string
	DocURL     string
	RetryAfter time.Duration
}

// Validate reports whether the failure is representable. Tool retry is an
// execution-policy decision, so durable Tool failures do not carry a retry
// delay in the current product contract.
func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("tool: unknown failure kind %q", f.Kind)
	}
	if f.RetryAfter < 0 {
		return errors.New("tool: failure retry delay must not be negative")
	}
	if f.RetryAfter != 0 {
		return errors.New("tool: failure must not carry a retry delay")
	}
	return nil
}
