// Package modelref defines an explicit provider/model selection. Executions and
// specialized runtime roles share the same invariant: a selection is either
// unset (use the surrounding default) or names both values; provider inference
// is deliberately unsupported.
package modelref

import (
	"encoding/json"
	"errors"
)

// ErrIncomplete reports a provider/model pair where only one value was set.
var ErrIncomplete = errors.New("model selection: provider and model must be set together")

// Selection is an immutable model choice. Its zero value asks the owning use
// case to use its configured default.
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
