package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidArguments reports malformed or non-object tool arguments.
	ErrInvalidArguments = errors.New("tool: invalid arguments")
	// ErrInvalidResult reports a tool result that cannot cross the JSON-based
	// provider and runtime protocol boundaries without losing information.
	ErrInvalidResult = errors.New("tool: invalid result")
)

// Arguments is an immutable, canonical tool argument object. Its zero value is
// an empty object. Keeping ownership and canonicalization here gives resume,
// deduplication, and cache identity one stable representation without exposing
// a mutable map to execution records.
type Arguments struct {
	raw string
}

// ParseArguments validates and canonicalizes a provider argument document.
// Empty input is the ergonomic no-arguments form; explicit null and all
// non-object values are rejected.
func ParseArguments(raw string) (Arguments, error) {
	if len(bytes.TrimSpace([]byte(raw))) == 0 {
		return Arguments{}, nil
	}
	var value map[string]any
	if err := decodeValue([]byte(raw), &value); err != nil {
		return Arguments{}, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	if value == nil {
		return Arguments{}, fmt.Errorf("%w: expected an object", ErrInvalidArguments)
	}
	return ArgumentsFromMap(value)
}

// ArgumentsFromMap takes an ownership-isolated snapshot of a wire object.
func ArgumentsFromMap(value map[string]any) (Arguments, error) {
	if value == nil {
		return Arguments{}, fmt.Errorf("%w: expected an object", ErrInvalidArguments)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Arguments{}, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	if string(encoded) == "{}" {
		return Arguments{}, nil
	}
	return Arguments{raw: string(encoded)}, nil
}

// Canonical returns the stable spelling used for resume and cache identity.
func (a Arguments) Canonical() string {
	if a.raw == "" {
		return "{}"
	}
	return a.raw
}

// Map returns a recursively ownership-isolated wire projection.
func (a Arguments) Map() map[string]any {
	var value map[string]any
	if err := decodeValue([]byte(a.Canonical()), &value); err != nil {
		panic(fmt.Sprintf("tool: corrupt Arguments invariant: %v", err))
	}
	return value
}

func (a Arguments) MarshalJSON() ([]byte, error) { return []byte(a.Canonical()), nil }

func (a *Arguments) UnmarshalJSON(data []byte) error {
	if a == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalidArguments)
	}
	parsed, err := ParseArguments(string(data))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// Result is an immutable, canonical value returned by a tool. A
// ToolInvocation uses *Result so nil means "not returned yet", while a present
// result may legitimately contain null.
type Result struct {
	raw string
}

// StringResult returns the infallible result value for text output.
func StringResult(value string) Result {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("tool: encode string result: %v", err))
	}
	return Result{raw: string(encoded)}
}

// NewResult snapshots and validates an arbitrary adapter or wire value.
func NewResult(value any) (Result, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalidResult, err)
	}
	return ParseResult(encoded)
}

// ParseResult validates and canonicalizes one complete result document.
func ParseResult(data []byte) (Result, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Result{}, fmt.Errorf("%w: empty JSON document", ErrInvalidResult)
	}
	var value any
	if err := decodeValue(data, &value); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalidResult, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Result{}, fmt.Errorf("%w: normalize: %w", ErrInvalidResult, err)
	}
	return Result{raw: string(encoded)}, nil
}

// Any returns a recursively ownership-isolated wire or presentation value.
func (r Result) Any() any {
	var value any
	if err := decodeValue([]byte(r.canonical()), &value); err != nil {
		panic(fmt.Sprintf("tool: corrupt Result invariant: %v", err))
	}
	return value
}

// String returns the contained string and whether this result is textual.
func (r Result) String() (string, bool) {
	var value string
	if err := json.Unmarshal([]byte(r.canonical()), &value); err != nil {
		return "", false
	}
	return value, true
}

func (r Result) canonical() string {
	if r.raw == "" {
		return "null"
	}
	return r.raw
}

func (r Result) MarshalJSON() ([]byte, error) { return []byte(r.canonical()), nil }

func (r *Result) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalidResult)
	}
	parsed, err := ParseResult(data)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// decodeValue preserves JSON numbers exactly and rejects trailing documents.
// Tool arguments can contain identifiers larger than IEEE-754's exact integer
// range; coercing them through float64 would silently change cache identity and
// exported transcripts.
func decodeValue(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
