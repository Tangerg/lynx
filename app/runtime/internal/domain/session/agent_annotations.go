package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidAgentAnnotations reports malformed delegated-session annotations.
// Agent annotations are an opaque extension point owned by agent/core, but the
// runtime still guarantees that their durable representation is a JSON object.
var ErrInvalidAgentAnnotations = errors.New("session: invalid agent annotations")

// AgentAnnotations is the immutable, JSON-object value carried by delegated
// agent sessions. Keeping the object opaque prevents agent-specific keys from
// leaking into Lyra's product Session model while preserving the
// core.SessionStore contract for child-agent resume.
//
// The zero value is the empty object.
type AgentAnnotations struct {
	object string
}

// ParseAgentAnnotations validates and owns a JSON object. Null, arrays, scalar
// values, and malformed JSON are rejected because core.Session.Metadata is
// documented as an object.
func ParseAgentAnnotations(data []byte) (AgentAnnotations, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return AgentAnnotations{}, fmt.Errorf("%w: empty JSON document", ErrInvalidAgentAnnotations)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return AgentAnnotations{}, fmt.Errorf("%w: %v", ErrInvalidAgentAnnotations, err)
	}
	if object == nil {
		return AgentAnnotations{}, fmt.Errorf("%w: expected a JSON object", ErrInvalidAgentAnnotations)
	}
	if len(object) == 0 {
		return AgentAnnotations{}, nil
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return AgentAnnotations{}, fmt.Errorf("%w: normalize object: %v", ErrInvalidAgentAnnotations, err)
	}
	return AgentAnnotations{object: string(normalized)}, nil
}

// JSON returns an ownership-isolated representation. Empty annotations always
// use {}, never null, so storage and the agent adapter share one canonical
// empty value.
func (a AgentAnnotations) JSON() []byte {
	if a.object == "" {
		return []byte("{}")
	}
	return []byte(a.object)
}

// String returns the canonical JSON object for persistence.
func (a AgentAnnotations) String() string {
	if a.object == "" {
		return "{}"
	}
	return a.object
}
