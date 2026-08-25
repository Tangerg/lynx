// Package jsonmetadata owns the JSON object representation used by vector
// stores that persist Core metadata in a native JSON column.
package jsonmetadata

import (
	"encoding/json"

	"github.com/Tangerg/lynx/core/metadata"
)

// Codec converts metadata to and from a JSON object. Its zero value is ready
// to use.
type Codec struct{}

// Marshal encodes metadata, representing a nil map as an empty JSON object.
func (Codec) Marshal(value metadata.Map) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

// Unmarshal decodes metadata, representing an absent value as a nil map.
func (Codec) Unmarshal(raw []byte) (metadata.Map, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value metadata.Map
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}
