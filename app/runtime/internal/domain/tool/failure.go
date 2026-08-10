package tool

import (
	"errors"
	"fmt"
	"time"
)

// FailureKind classifies why one Tool invocation did not produce a successful
// result. It is deliberately separate from Run failure taxonomy.
type FailureKind uint8

const (
	FailureInternal FailureKind = iota
	FailureDenied
	FailureExecution
	FailureChildRunCanceled
)

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
func (failure Failure) Validate() error {
	if failure.Kind > FailureChildRunCanceled {
		return fmt.Errorf("tool: unknown failure kind %d", failure.Kind)
	}
	if failure.RetryAfter < 0 {
		return errors.New("tool: failure retry delay must not be negative")
	}
	if failure.RetryAfter != 0 {
		return errors.New("tool: failure must not carry a retry delay")
	}
	return nil
}
