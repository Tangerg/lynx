package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ToolArgumentOverride is a validated, immutable replacement for one pending
// tool call's argument object. It is deliberately a value object instead of an
// exported map so interaction drafts, durable outbox entries, and adapters
// cannot share mutable approval state.
type ToolArgumentOverride struct {
	encoded []byte
}

// ParseToolArgumentOverride accepts exactly one non-empty JSON object. Duplicate
// keys are rejected because their last-value-wins decoding would make the
// reviewed text and the executed argument object disagree.
func ParseToolArgumentOverride(encoded []byte) (*ToolArgumentOverride, error) {
	value, err := decodeToolArgumentJSON(encoded)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("tool argument override must be a JSON object")
	}
	if len(object) == 0 {
		return nil, errors.New("tool argument override must contain at least one argument")
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode tool argument override: %w", err)
	}
	return &ToolArgumentOverride{encoded: normalized}, nil
}

func (t *ToolArgumentOverride) Validate() error {
	if t == nil {
		return errors.New("tool argument override is nil")
	}
	validated, err := ParseToolArgumentOverride(t.encoded)
	if err != nil {
		return err
	}
	if !bytes.Equal(validated.encoded, t.encoded) {
		return errors.New("tool argument override is not normalized")
	}
	return nil
}

func (t *ToolArgumentOverride) Clone() *ToolArgumentOverride {
	if t == nil {
		return nil
	}
	return &ToolArgumentOverride{encoded: bytes.Clone(t.encoded)}
}

func (t *ToolArgumentOverride) Equal(other *ToolArgumentOverride) bool {
	if t == nil || other == nil {
		return t == other
	}
	return bytes.Equal(t.encoded, other.encoded)
}

// JSON returns a detached normalized representation for editors and durable
// projections.
func (t *ToolArgumentOverride) JSON() []byte {
	if t == nil {
		return nil
	}
	return bytes.Clone(t.encoded)
}

// Object returns a detached protocol-ready object without reducing JSON
// numbers to float64.
func (t *ToolArgumentOverride) Object() (map[string]any, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	value, err := decodeToolArgumentJSON(t.encoded)
	if err != nil {
		return nil, err
	}
	return value.(map[string]any), nil
}

func (t ToolArgumentOverride) MarshalJSON() ([]byte, error) {
	if err := (&t).Validate(); err != nil {
		return nil, err
	}
	return bytes.Clone(t.encoded), nil
}

func (t *ToolArgumentOverride) UnmarshalJSON(encoded []byte) error {
	parsed, err := ParseToolArgumentOverride(encoded)
	if err != nil {
		return err
	}
	*t = *parsed
	return nil
}

func decodeToolArgumentJSON(encoded []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	value, err := decodeDistinctJSONValue(decoder, "arguments")
	if err != nil {
		return nil, fmt.Errorf("tool argument override: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("tool argument override contains more than one JSON value")
		}
		return nil, fmt.Errorf("tool argument override: %w", err)
	}
	return value, nil
}

func decodeDistinctJSONValue(decoder *json.Decoder, path string) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("%s has a non-string object key", path)
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("%s repeats key %q", path, key)
			}
			value, err := decodeDistinctJSONValue(decoder, path+"."+key)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		values := make([]any, 0)
		for index := 0; decoder.More(); index++ {
			value, err := decodeDistinctJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%s starts with unexpected delimiter %q", path, delimiter)
	}
}
