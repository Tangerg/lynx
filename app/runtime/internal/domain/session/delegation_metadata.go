package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidDelegationMetadata reports malformed delegated-session continuation
// metadata.
var ErrInvalidDelegationMetadata = errors.New("session: invalid delegation metadata")

// DelegationMetadata is immutable, opaque JSON-object metadata that a delegated
// conversation needs in order to resume. The Session domain preserves it as a
// durable continuation value without interpreting producer-specific keys.
//
// The zero value is the empty object.
type DelegationMetadata struct {
	object string
}

// ParseDelegationMetadata validates and owns a JSON object. Null, arrays,
// scalar values, and malformed JSON are rejected because continuation metadata
// is always an object.
func ParseDelegationMetadata(data []byte) (DelegationMetadata, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return DelegationMetadata{}, fmt.Errorf("%w: empty JSON document", ErrInvalidDelegationMetadata)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return DelegationMetadata{}, fmt.Errorf("%w: decode object: %w", ErrInvalidDelegationMetadata, err)
	}
	if object == nil {
		return DelegationMetadata{}, fmt.Errorf("%w: expected a JSON object", ErrInvalidDelegationMetadata)
	}
	if len(object) == 0 {
		return DelegationMetadata{}, nil
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return DelegationMetadata{}, fmt.Errorf("%w: normalize object: %w", ErrInvalidDelegationMetadata, err)
	}
	return DelegationMetadata{object: string(normalized)}, nil
}

// JSON returns an ownership-isolated representation. Empty metadata always
// uses {}, never null, so storage and continuation adapters share one canonical
// empty value.
func (m DelegationMetadata) JSON() []byte {
	if m.object == "" {
		return []byte("{}")
	}
	return []byte(m.object)
}

// String returns the canonical JSON object for persistence.
func (m DelegationMetadata) String() string {
	if m.object == "" {
		return "{}"
	}
	return m.object
}
