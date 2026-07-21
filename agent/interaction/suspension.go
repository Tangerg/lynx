package interaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

const SuspensionSchemaVersion uint16 = 1

var (
	ErrInvalidSuspension  = errors.New("interaction: invalid suspension")
	ErrSuspended          = errors.New("interaction: suspended")
	ErrSuspensionConflict = errors.New("interaction: suspension conflict")
	ErrSuspensionStale    = errors.New("interaction: stale suspension")
)

// SuspensionKind identifies who owns the resumable boundary.
type SuspensionKind string

const (
	SuspensionHuman SuspensionKind = "human"
	SuspensionTool  SuspensionKind = "tool"
)

func (k SuspensionKind) Valid() bool {
	return k == SuspensionHuman || k == SuspensionTool
}

// Suspension is the complete JSON-safe state exposed when a process waits for
// external input. Payload is framework-owned opaque continuation state;
// Prompt and ResumeSchema are host-facing protocol values.
type Suspension struct {
	SchemaVersion uint16          `json:"schema_version"`
	ID            string          `json:"id"`
	Kind          SuspensionKind  `json:"kind"`
	Prompt        json.RawMessage `json:"prompt"`
	ResumeSchema  json.RawMessage `json:"resume_schema"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Response      json.RawMessage `json:"response,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	RespondedAt   time.Time       `json:"responded_at,omitzero"`
}

func (s Suspension) Validate() error {
	if s.SchemaVersion != SuspensionSchemaVersion {
		return fmt.Errorf("%w: schema version %d is unsupported", ErrInvalidSuspension, s.SchemaVersion)
	}
	if err := ValidateID(s.ID); err != nil {
		return fmt.Errorf("%w: ID: %w", ErrInvalidSuspension, err)
	}
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidSuspension, s.Kind)
	}
	if s.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at must not be zero", ErrInvalidSuspension)
	}
	if !validJSON(s.Prompt) {
		return fmt.Errorf("%w: prompt must be valid JSON", ErrInvalidSuspension)
	}
	if err := validateSchema(s.ResumeSchema); err != nil {
		return fmt.Errorf("%w: resume_schema: %w", ErrInvalidSuspension, err)
	}
	if len(s.Payload) > 0 && !validJSON(s.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidSuspension)
	}
	if len(s.Response) == 0 {
		if !s.RespondedAt.IsZero() {
			return fmt.Errorf("%w: responded_at requires a response", ErrInvalidSuspension)
		}
		return nil
	}
	if !validJSON(s.Response) {
		return fmt.Errorf("%w: response must be valid JSON", ErrInvalidSuspension)
	}
	if s.RespondedAt.IsZero() {
		return fmt.Errorf("%w: response requires responded_at", ErrInvalidSuspension)
	}
	if s.RespondedAt.Before(s.CreatedAt) {
		return fmt.Errorf("%w: responded_at must not precede created_at", ErrInvalidSuspension)
	}
	if _, err := s.ValidateResponse(s.Response); err != nil {
		return fmt.Errorf("%w: stored response: %w", ErrInvalidSuspension, err)
	}
	return nil
}

func (s Suspension) Responded() bool { return len(s.Response) > 0 }

func (s Suspension) Clone() *Suspension {
	cloned := s
	cloned.Prompt = bytes.Clone(s.Prompt)
	cloned.ResumeSchema = bytes.Clone(s.ResumeSchema)
	cloned.Payload = bytes.Clone(s.Payload)
	cloned.Response = bytes.Clone(s.Response)
	return &cloned
}

func (s Suspension) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	type wire Suspension
	return json.Marshal(wire(s))
}

func (s *Suspension) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSuspension)
	}
	type wire Suspension
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidSuspension, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalidSuspension)
	}
	candidate := Suspension(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// ValidateResponse converts response to its canonical JSON representation and
// validates that value against ResumeSchema.
func (s Suspension) ValidateResponse(response any) (json.RawMessage, error) {
	canonical, value, err := canonicalJSON(response)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(s.ResumeSchema, &schema); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve schema: %w", err)
	}
	if err := resolved.Validate(value); err != nil {
		return nil, fmt.Errorf("response does not match schema: %w", err)
	}
	return canonical, nil
}

// SameResponse reports semantic JSON equality, independent of object key order
// and insignificant whitespace.
func (s Suspension) SameResponse(response any) bool {
	if !s.Responded() {
		return false
	}
	want, _, err := canonicalJSON(s.Response)
	if err != nil {
		return false
	}
	got, _, err := canonicalJSON(response)
	return err == nil && bytes.Equal(want, got)
}

// SuspendedError transports only durable suspension data across action and
// tool boundaries. It contains no callback, handler, or executable state.
type SuspendedError struct {
	Suspension Suspension
}

func (e *SuspendedError) Error() string {
	if e == nil {
		return ErrSuspended.Error()
	}
	return fmt.Sprintf("%s at %q", ErrSuspended, e.Suspension.ID)
}

func (e *SuspendedError) Unwrap() error { return ErrSuspended }

func validJSON(data json.RawMessage) bool {
	return len(data) > 0 && json.Valid(data)
}

func validateSchema(data json.RawMessage) error {
	if !validJSON(data) {
		return errors.New("must be valid JSON")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return err
	}
	_, err := schema.Resolve(nil)
	return err
}

func canonicalJSON(value any) (json.RawMessage, any, error) {
	var data []byte
	var err error
	switch value := value.(type) {
	case json.RawMessage:
		data = bytes.Clone(value)
	default:
		data, err = json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, errors.New("multiple JSON values")
		}
		return nil, nil, err
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, nil, err
	}
	return canonical, decoded, nil
}
