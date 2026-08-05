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

// SuspensionSchemaVersion is the only suspension shape this build accepts.
// [Suspension.Validate] rejects every other version rather than migrating it: a
// parked process resumes into running framework code, so accepting a shape this
// build does not understand would continue execution from state it cannot
// interpret.
const SuspensionSchemaVersion uint16 = 7

var (
	ErrInvalidSuspension  = errors.New("interaction: invalid suspension")
	ErrSuspended          = errors.New("interaction: suspended")
	ErrSuspensionConflict = errors.New("interaction: suspension conflict")
	ErrSuspensionStale    = errors.New("interaction: stale suspension")
)

// Suspension is the complete JSON-safe state exposed when a process waits for
// external input. It does not classify who must answer: the framework never
// branches on that, and the waiting side's own Prompt says what is being asked.
// FrameworkState is opaque execution state owned exclusively by the Agent
// runtime. Prompt and ResponseSchema are host-facing protocol values:
// whatever the waiting side needs to decide travels in Prompt, so a suspension
// carries no second, framework-defined slot for producer metadata.
type Suspension struct {
	SchemaVersion  uint16          `json:"schema_version"`
	ID             string          `json:"id"`
	Prompt         json.RawMessage `json:"prompt"`
	ResponseSchema json.RawMessage `json:"response_schema"`
	FrameworkState json.RawMessage `json:"framework_state,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Validate reports whether s is a suspension this build can resume from,
// wrapping [ErrInvalidSuspension]. A stored Response is checked against
// ResponseSchema as well, so a suspension is never valid while carrying an answer
// its own schema would reject.
//
// Both JSON boundaries call it, which is what keeps an invalid suspension from
// being persisted or accepted.
func (s Suspension) Validate() error {
	if s.SchemaVersion != SuspensionSchemaVersion {
		return fmt.Errorf("%w: schema version %d is unsupported", ErrInvalidSuspension, s.SchemaVersion)
	}
	if err := ValidateID(s.ID); err != nil {
		return fmt.Errorf("%w: ID: %w", ErrInvalidSuspension, err)
	}
	if s.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at must not be zero", ErrInvalidSuspension)
	}
	if !validJSON(s.Prompt) {
		return fmt.Errorf("%w: prompt must be valid JSON", ErrInvalidSuspension)
	}
	if err := validateSchema(s.ResponseSchema); err != nil {
		return fmt.Errorf("%w: response_schema: %w", ErrInvalidSuspension, err)
	}
	if len(s.FrameworkState) > 0 && !validJSON(s.FrameworkState) {
		return fmt.Errorf("%w: framework_state must be valid JSON", ErrInvalidSuspension)
	}
	if len(s.Response) == 0 {
		return nil
	}
	if !validJSON(s.Response) {
		return fmt.Errorf("%w: response must be valid JSON", ErrInvalidSuspension)
	}
	if _, err := s.ValidateResponse(s.Response); err != nil {
		return fmt.Errorf("%w: stored response: %w", ErrInvalidSuspension, err)
	}
	return nil
}

// Responded reports whether an answer has been recorded. It is derived from the
// payload rather than tracked in a flag, so a resumed process cannot disagree
// with the response it actually carries.
func (s Suspension) Responded() bool { return len(s.Response) > 0 }

// Clone returns an independently owned copy. The raw JSON fields are byte
// slices, so an assignment alone would leave the copy sharing wire bytes with
// state the runtime persists.
//
// It returns a pointer because a process holds *Suspension: handing back a value
// would make every caller take its address, and an accidental copy of that value
// is the aliasing this exists to prevent.
func (s Suspension) Clone() *Suspension {
	cloned := s
	cloned.Prompt = bytes.Clone(s.Prompt)
	cloned.ResponseSchema = bytes.Clone(s.ResponseSchema)
	cloned.FrameworkState = bytes.Clone(s.FrameworkState)
	cloned.Response = bytes.Clone(s.Response)
	return &cloned
}

// MarshalJSON refuses to encode a suspension that would not validate, so an
// unusable one can never reach a store.
func (s Suspension) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	type wire Suspension
	return json.Marshal(wire(s))
}

// UnmarshalJSON accepts only an exact, complete suspension: an unknown field or
// a trailing value is a version skew, and quietly dropping either would resume a
// process from less state than whoever wrote it believed was saved. The decoded
// value is validated before it replaces the receiver, so a failed decode leaves
// s untouched.
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
// validates that value against ResponseSchema.
func (s Suspension) ValidateResponse(response any) (json.RawMessage, error) {
	canonical, value, err := canonicalJSON(response)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(s.ResponseSchema, &schema); err != nil {
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

// SuspendedError transports only snapshot-compatible suspension data across action and
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

// Unwrap reports [ErrSuspended] so a caller can test for a park with
// errors.Is. There is no underlying failure to expose — the suspension is a
// control-flow signal, and its data travels in the field, not in the error
// chain.
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
