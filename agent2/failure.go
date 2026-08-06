package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	maxFailureCodeBytes    = 128
	maxFailureMessageBytes = 4096
)

// ErrInvalidFailure reports a malformed stable execution failure fact.
var ErrInvalidFailure = errors.New("agent: invalid failure")

// FailureKind is the stable framework-level classification of a failed
// Process. It deliberately does not imply retryability or business semantics.
type FailureKind uint8

const (
	// FailureKindInvalid is the invalid zero value.
	FailureKindInvalid FailureKind = iota
	// FailureKindExecution identifies an ordinary Strategy execution failure.
	FailureKindExecution
	// FailureKindContract identifies a violated Framework or Strategy contract.
	FailureKindContract
	// FailureKindExternal identifies failed external infrastructure.
	FailureKindExternal
	// FailureKindPanic identifies a recovered panic at an execution boundary.
	FailureKindPanic
)

// String returns the stable failure-kind name.
func (kind FailureKind) String() string {
	switch kind {
	case FailureKindExecution:
		return "execution"
	case FailureKindContract:
		return "contract"
	case FailureKindExternal:
		return "external"
	case FailureKindPanic:
		return "panic"
	default:
		return "invalid"
	}
}

func parseFailureKind(value string) (FailureKind, error) {
	switch value {
	case "execution":
		return FailureKindExecution, nil
	case "contract":
		return FailureKindContract, nil
	case "external":
		return FailureKindExternal, nil
	case "panic":
		return FailureKindPanic, nil
	default:
		return FailureKindInvalid, fmt.Errorf("%w: unknown kind %q", ErrInvalidFailure, value)
	}
}

// Failure is an immutable, snapshot-safe classification and explanation. Code
// is stable for machine decisions; Message is diagnostic and must not contain
// secrets or unbounded external payloads.
type Failure struct {
	kind    FailureKind
	code    string
	message string
}

// NewFailure validates a stable failed-execution fact.
func NewFailure(kind FailureKind, code, message string) (Failure, error) {
	if kind < FailureKindExecution || kind > FailureKindPanic {
		return Failure{}, fmt.Errorf("%w: kind is required", ErrInvalidFailure)
	}
	if !validQualifiedName(code) || len(code) > maxFailureCodeBytes {
		return Failure{}, fmt.Errorf("%w: code must be a lowercase qualified name containing at most %d bytes", ErrInvalidFailure, maxFailureCodeBytes)
	}
	if message == "" || strings.TrimSpace(message) != message || len(message) > maxFailureMessageBytes {
		return Failure{}, fmt.Errorf("%w: message must be non-empty, trimmed, and at most %d bytes", ErrInvalidFailure, maxFailureMessageBytes)
	}
	return Failure{kind: kind, code: code, message: message}, nil
}

// Kind returns the framework-level failure classification.
func (f Failure) Kind() FailureKind { return f.kind }

// Code returns the stable machine-readable reason.
func (f Failure) Code() string { return f.code }

// Message returns the bounded diagnostic explanation.
func (f Failure) Message() string { return f.message }

// Valid reports whether the Failure was constructed successfully.
func (f Failure) Valid() bool {
	return f.kind >= FailureKindExecution && f.kind <= FailureKindPanic &&
		validQualifiedName(f.code) && f.message != ""
}

// MarshalJSON returns the validated portable failure fact.
func (f Failure) MarshalJSON() ([]byte, error) {
	if !f.Valid() {
		return nil, ErrInvalidFailure
	}
	return json.Marshal(failureWire{Kind: f.kind.String(), Code: f.code, Message: f.message})
}

// UnmarshalJSON replaces f with a strictly decoded Failure.
func (f *Failure) UnmarshalJSON(data []byte) error {
	if f == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidFailure)
	}
	var wire failureWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidFailure, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFailure, err)
	}
	kind, err := parseFailureKind(wire.Kind)
	if err != nil {
		return err
	}
	value, err := NewFailure(kind, wire.Code, wire.Message)
	if err != nil {
		return err
	}
	*f = value
	return nil
}

type failureWire struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
