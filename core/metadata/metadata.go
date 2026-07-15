package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrNilMap reports an attempt to write through a nil Map.
	ErrNilMap = errors.New("metadata: nil map")
	// ErrEmptyKey reports an empty metadata key.
	ErrEmptyKey = errors.New("metadata: empty key")
	// ErrInvalidValue reports a value that is not valid JSON.
	ErrInvalidValue = errors.New("metadata: invalid JSON value")
)

// Map stores metadata values in their encoded JSON representation.
//
// Keeping values encoded prevents runtime-only objects such as functions,
// readers, and SDK clients from entering protocol DTOs unnoticed.
type Map map[string]json.RawMessage

// New returns an initialized, empty Map.
func New() Map {
	return make(Map)
}

// FromValues encodes an ordinary value map into a JSON-safe Map.
func FromValues(values map[string]any) (Map, error) {
	if values == nil {
		return nil, nil
	}
	encoded := make(Map, len(values))
	for key, value := range values {
		if err := Set(encoded, key, value); err != nil {
			return nil, err
		}
	}
	return encoded, nil
}

// Values decodes every entry into ordinary Go JSON values for SDK and storage
// boundaries that do not accept json.RawMessage values.
func (m Map) Values() (map[string]any, error) {
	if m == nil {
		return nil, nil
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	values := make(map[string]any, len(m))
	for key, raw := range m {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("metadata: decode %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}

// Set encodes value as JSON and stores it under key.
//
// Set fails immediately when value cannot be represented as JSON. Callers
// that have a nil Map should initialize it with [New] first.
func Set(m Map, key string, value any) error {
	if m == nil {
		return ErrNilMap
	}
	if key == "" {
		return ErrEmptyKey
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metadata: encode %q: %w", key, err)
	}
	m[key] = encoded
	return nil
}

// Decode decodes the value stored under key into T. The boolean reports
// whether key was present.
func Decode[T any](m Map, key string) (T, bool, error) {
	var zero T
	if key == "" {
		return zero, false, ErrEmptyKey
	}
	raw, ok := m[key]
	if !ok {
		return zero, false, nil
	}
	if !json.Valid(raw) {
		return zero, true, fmt.Errorf("metadata: decode %q: %w", key, ErrInvalidValue)
	}

	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return zero, true, fmt.Errorf("metadata: decode %q: %w", key, err)
	}
	return value, true, nil
}

// Clone returns a deep copy of m. A nil Map remains nil.
func (m Map) Clone() Map {
	if m == nil {
		return nil
	}
	clone := make(Map, len(m))
	for key, value := range m {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}

// Validate reports empty keys and values that are not complete JSON values.
func (m Map) Validate() error {
	for key, value := range m {
		if key == "" {
			return ErrEmptyKey
		}
		if !json.Valid(value) {
			return fmt.Errorf("metadata: key %q: %w", key, ErrInvalidValue)
		}
	}
	return nil
}

// MarshalJSON validates m before writing its JSON object representation.
func (m Map) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type wireMap Map
	return json.Marshal(wireMap(m))
}

// UnmarshalJSON decodes and validates a JSON metadata object.
func (m *Map) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("metadata: unmarshal into nil Map pointer")
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("metadata: decode map: %w", err)
	}
	candidate := Map(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}
