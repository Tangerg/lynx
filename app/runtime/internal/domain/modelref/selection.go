// Package modelref defines an explicit provider/model selection and the
// immutable token-limit envelope attached to that exact identity. Executions
// and specialized runtime roles share the same invariant: a selection is
// either unset (use the surrounding default) or names both values; provider
// inference is deliberately unsupported.
package modelref

import (
	"errors"
	"strings"
)

// ErrIncomplete reports a provider/model pair where only one value was set.
var ErrIncomplete = errors.New("model selection: provider and model must be set together")

// ErrSurroundingWhitespace reports a provider/model identity that would compare
// differently before and after ordinary input normalization.
var ErrSurroundingWhitespace = errors.New("model selection: provider and model must not have surrounding whitespace")

// Selection is an immutable model choice. Its zero value asks the owning use
// case to use its configured default.
type Selection struct {
	provider string
	model    string
}

// New constructs a selection from its provider and model identities.
func New(provider, model string) (Selection, error) {
	if (provider == "") != (model == "") {
		return Selection{}, ErrIncomplete
	}
	if provider != strings.TrimSpace(provider) || model != strings.TrimSpace(model) {
		return Selection{}, ErrSurroundingWhitespace
	}
	return Selection{provider: provider, model: model}, nil
}

// Validate documents the zero-or-complete invariant at aggregate boundaries.
// Selection is immutable, so values constructed by New already satisfy it.
func (s Selection) Validate() error {
	_, err := New(s.provider, s.model)
	return err
}

// Configured reports whether s pins one provider and model.
func (s Selection) Configured() bool { return s.model != "" }

// Provider returns the explicitly selected provider, or "" for the runtime default.
func (s Selection) Provider() string { return s.provider }

// Model returns the explicitly selected model, or "" for the runtime default.
func (s Selection) Model() string { return s.model }
