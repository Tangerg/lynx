// Package modelref defines the explicit provider/model selection used by one
// execution. A selection is either unset (the runtime default) or names both
// a provider and a model; provider inference is deliberately unsupported.
package modelref

import (
	"encoding/json"
	"errors"
)

// ErrIncomplete reports a provider/model pair where only one value was set.
var ErrIncomplete = errors.New("model selection: provider and model must be set together")

// Selection is an immutable per-execution model choice. Its zero value is the
// unset selection, which asks the runtime to use its configured default.
type Selection struct {
	provider string
	model    string
}

// New constructs a selection from its protocol values.
func New(provider, model string) (Selection, error) {
	if (provider == "") != (model == "") {
		return Selection{}, ErrIncomplete
	}
	return Selection{provider: provider, model: model}, nil
}

// Configured reports whether s pins one provider and model.
func (s Selection) Configured() bool { return s.model != "" }

// Provider returns the explicitly selected provider, or "" for the runtime default.
func (s Selection) Provider() string { return s.provider }

// Model returns the explicitly selected model, or "" for the runtime default.
func (s Selection) Model() string { return s.model }

// MarshalJSON preserves the selection as ordinary protocol values for durable
// payloads while keeping the Go representation immutable.
func (s Selection) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"provider": s.provider, "model": s.model})
}

// UnmarshalJSON validates a durable selection before it enters a domain value.
func (s *Selection) UnmarshalJSON(data []byte) error {
	var encoded map[string]string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	selection, err := New(encoded["provider"], encoded["model"])
	if err != nil {
		return err
	}
	*s = selection
	return nil
}
