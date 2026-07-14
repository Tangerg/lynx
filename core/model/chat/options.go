package chat

import (
	"errors"
	"maps"
	"slices"
)

// Options holds the per-request configuration LLM providers accept.
// Standard parameters (model id, temperature, ...) are typed; anything a
// specific provider needs is stored in Extra. Pointer-typed fields use nil
// to mean "not set"; the provider applies its own default.
type Options struct {
	// Model is the provider model identifier (e.g. "gpt-4o", "claude-3-5-sonnet").
	Model string `json:"model"`

	// FrequencyPenalty discourages repeated tokens. Range -2.0 to 2.0.
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`

	// MaxTokens caps the number of tokens the model may generate.
	MaxTokens *int64 `json:"max_tokens,omitempty"`

	// PresencePenalty discourages already-mentioned tokens. Range -2.0 to 2.0.
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`

	// Stop terminates generation when any sequence is produced.
	Stop []string `json:"stop,omitzero"`

	// Temperature controls sampling randomness. Range 0.0 to 2.0.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopK limits sampling to the K highest-probability tokens.
	TopK *int64 `json:"top_k,omitempty"`

	// TopP enables nucleus sampling using the cumulative probability mass.
	TopP *float64 `json:"top_p,omitempty"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error if
// model is empty — every provider requires a model id.
//
// Example:
//
//	opts, err := chat.NewOptions("gpt-4o")
//	if err != nil { return err }
//	opts.Set("response_format", map[string]any{"type": "json_object"})
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("chat.NewOptions: model id must not be empty")
	}

	return &Options{
		Model: model,
	}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	if o == nil || o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone returns a deep copy of Options. Pointer fields, slices, and the
// Extra map are duplicated so the result is safe to mutate independently.
// A nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}

	return &Options{
		Model:            o.Model,
		FrequencyPenalty: clonePointer(o.FrequencyPenalty),
		MaxTokens:        clonePointer(o.MaxTokens),
		PresencePenalty:  clonePointer(o.PresencePenalty),
		Stop:             slices.Clone(o.Stop),
		Temperature:      clonePointer(o.Temperature),
		TopK:             clonePointer(o.TopK),
		TopP:             clonePointer(o.TopP),
		Extra:            maps.Clone(o.Extra),
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-empty values overwrite; slices append; the Extra map merges
// last-write-wins.
//
// Returns an error when base is nil so callers don't accidentally produce
// an Options without a model id.
//
// Example:
//
//	merged, err := chat.MergeOptions(modelDefaults, requestOverrides)
//	if err != nil { return err }
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("chat.MergeOptions: base options must not be nil")
	}

	// Deep-clone (not ptr.Clone, which is shallow): a subsequent
	// applyOverride does maps.Copy into merged.Extra, so a shallow clone
	// would write through into the caller's base.Extra. Clone matches every
	// other modality's MergeOptions and honors this function's "clones base"
	// contract.
	merged := base.Clone()
	if len(overrides) == 0 {
		return merged, nil
	}

	merged.ensureExtra()

	for _, override := range overrides {
		if override == nil {
			continue
		}
		merged.applyOverride(override)
	}
	return merged, nil
}

// applyOverride mutates the receiver in place with the non-zero fields of
// src. Extracted from MergeOptions to keep the merge body free of repeated
// "if-not-zero overwrite" boilerplate.
func (o *Options) applyOverride(src *Options) {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.FrequencyPenalty != nil {
		o.FrequencyPenalty = src.FrequencyPenalty
	}
	if src.MaxTokens != nil {
		o.MaxTokens = src.MaxTokens
	}
	if src.PresencePenalty != nil {
		o.PresencePenalty = src.PresencePenalty
	}
	if len(src.Stop) > 0 {
		// Replace, not append: every other scalar field overrides on
		// non-zero, and appending makes MergeOptions non-idempotent —
		// merging the same override N times would multiply stop
		// sequences. Clone so callers can mutate either slice safely.
		o.Stop = slices.Clone(src.Stop)
	}
	if src.Temperature != nil {
		o.Temperature = src.Temperature
	}
	if src.TopK != nil {
		o.TopK = src.TopK
	}
	if src.TopP != nil {
		o.TopP = src.TopP
	}
	if len(src.Extra) > 0 {
		maps.Copy(o.Extra, src.Extra)
	}
}
