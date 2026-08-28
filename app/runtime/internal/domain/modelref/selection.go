// Package modelref defines an explicit provider/model/reasoning selection and
// the immutable token-limit envelope attached to that exact identity.
// Executions and specialized runtime roles share the same invariant: a
// selection is either unset (use the surrounding default) or names provider
// and model together; reasoning effort is optional and belongs to that exact
// model. Provider inference is deliberately unsupported.
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

// ErrReasoningEffortWithoutModel reports a reasoning choice that has no exact
// provider/model identity to interpret its model-owned vocabulary.
var ErrReasoningEffortWithoutModel = errors.New("model selection: reasoning effort requires provider and model")

// ErrReasoningEffortWhitespace reports a reasoning identity that would compare
// differently before and after ordinary input normalization.
var ErrReasoningEffortWhitespace = errors.New("model selection: reasoning effort must not have surrounding whitespace")

// ErrUnsupported reports a syntactically valid exact selection the configured
// Runtime cannot admit.
var ErrUnsupported = errors.New("model selection: unsupported")

// IsInvalid reports whether err is a stable model-selection syntax failure.
func IsInvalid(err error) bool {
	return errors.Is(err, ErrIncomplete) ||
		errors.Is(err, ErrSurroundingWhitespace) ||
		errors.Is(err, ErrReasoningEffortWithoutModel) ||
		errors.Is(err, ErrReasoningEffortWhitespace)
}

// Selection is an immutable model choice. Its zero value asks the owning use
// case to use its configured default.
type Selection struct {
	provider        string
	model           string
	reasoningEffort string
}

// Patch describes an atomic edit of one model selection. Provider and model
// form one identity and must therefore be supplied together. Changing that
// identity without an explicit effort resets effort to the target model's
// provider default; changing effort alone retains the current identity.
type Patch struct {
	Provider        *string
	Model           *string
	ReasoningEffort *string
}

// Empty reports whether p requests no selection edit.
func (p Patch) Empty() bool {
	return p.Provider == nil && p.Model == nil && p.ReasoningEffort == nil
}

// Apply returns the exact selection produced by p.
func (p Patch) Apply(current Selection) (Selection, error) {
	if (p.Provider == nil) != (p.Model == nil) {
		return Selection{}, ErrIncomplete
	}
	provider, model, effort := current.Provider(), current.Model(), current.ReasoningEffort()
	if p.Provider != nil {
		provider, model = *p.Provider, *p.Model
		effort = ""
	}
	if p.ReasoningEffort != nil {
		effort = *p.ReasoningEffort
	}
	return NewWithReasoningEffort(provider, model, effort)
}

// New constructs a selection from its provider and model identities.
func New(provider, model string) (Selection, error) {
	return NewWithReasoningEffort(provider, model, "")
}

// NewWithReasoningEffort constructs one exact execution selection. An empty
// effort leaves intensity to the selected model's provider default; a non-empty
// value is interpreted only against that exact model's advertised vocabulary.
func NewWithReasoningEffort(provider, model, reasoningEffort string) (Selection, error) {
	if (provider == "") != (model == "") {
		return Selection{}, ErrIncomplete
	}
	if provider != strings.TrimSpace(provider) || model != strings.TrimSpace(model) {
		return Selection{}, ErrSurroundingWhitespace
	}
	if reasoningEffort != strings.TrimSpace(reasoningEffort) {
		return Selection{}, ErrReasoningEffortWhitespace
	}
	if model == "" && reasoningEffort != "" {
		return Selection{}, ErrReasoningEffortWithoutModel
	}
	return Selection{provider: provider, model: model, reasoningEffort: reasoningEffort}, nil
}

// Validate documents the zero-or-complete invariant at aggregate boundaries.
// Selection is immutable, so values constructed by New already satisfy it.
func (s Selection) Validate() error {
	_, err := NewWithReasoningEffort(s.provider, s.model, s.reasoningEffort)
	return err
}

// Configured reports whether s pins one provider and model.
func (s Selection) Configured() bool { return s.model != "" }

// Provider returns the explicitly selected provider, or "" for the runtime default.
func (s Selection) Provider() string { return s.provider }

// Model returns the explicitly selected model, or "" for the runtime default.
func (s Selection) Model() string { return s.model }

// ReasoningEffort returns the selected model's explicit intensity, or "" to
// use that model's provider default.
func (s Selection) ReasoningEffort() string { return s.reasoningEffort }
