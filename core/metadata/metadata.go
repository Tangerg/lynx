package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	// ErrNilMap reports a Set through a nil *Map pointer. A nil Map VALUE is
	// fine — Set initializes it in place.
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

// FromValues encodes an ordinary value map into a JSON-safe Map.
func FromValues(values map[string]any) (Map, error) {
	if values == nil {
		return nil, nil
	}
	encoded := make(Map, len(values))
	for key, value := range values {
		if err := encoded.Set(key, value); err != nil {
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
		value, err := decodeValue(raw)
		if err != nil {
			return nil, fmt.Errorf("metadata: decode %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}

func decodeValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeNumbers(value)
}

func normalizeNumbers(value any) (any, error) {
	switch value := value.(type) {
	case json.Number:
		return normalizeNumber(value)
	case []any:
		for i := range value {
			normalized, err := normalizeNumbers(value[i])
			if err != nil {
				return nil, err
			}
			value[i] = normalized
		}
	case map[string]any:
		for key, item := range value {
			normalized, err := normalizeNumbers(item)
			if err != nil {
				return nil, err
			}
			value[key] = normalized
		}
	}
	return value, nil
}

func normalizeNumber(number json.Number) (any, error) {
	text := number.String()
	if !strings.ContainsAny(text, ".eE") {
		if strings.HasPrefix(text, "-") {
			if value, err := strconv.ParseInt(text, 10, 64); err == nil {
				return value, nil
			}
		} else if value, err := strconv.ParseUint(text, 10, 64); err == nil {
			if value <= math.MaxInt64 {
				return int64(value), nil
			}
			return value, nil
		}
		return number, nil
	}
	value, err := number.Float64()
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Set encodes value as JSON and stores it under key, initializing a nil map
// in place — so a zero-value `Extra metadata.Map` field is writable without a
// prior initialization. Set fails immediately when value cannot be represented
// as JSON.
func (m *Map) Set(key string, value any) error {
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
	if *m == nil {
		*m = make(Map, 1)
	}
	(*m)[key] = encoded
	return nil
}

// Merge copies every entry from source into m. Source values overwrite entries
// with the same key and are deep-copied so later mutations cannot cross the
// metadata boundary. The operation validates both maps before mutating m.
func (m *Map) Merge(source Map) error {
	if m == nil {
		return ErrNilMap
	}
	if err := (*m).Validate(); err != nil {
		return fmt.Errorf("metadata: merge target: %w", err)
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("metadata: merge source: %w", err)
	}
	if len(source) == 0 {
		return nil
	}
	if *m == nil {
		*m = make(Map, len(source))
	}
	for key, value := range source {
		(*m)[key] = append(json.RawMessage(nil), value...)
	}
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
