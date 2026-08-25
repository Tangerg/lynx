package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/internal/extension"
	"github.com/Tangerg/lynx/core/internal/ptr"
	"github.com/Tangerg/lynx/core/metadata"
)

var ErrInvalidOptions = errors.New("chat: invalid options")

// Options contains provider-neutral per-request generation overrides. Its zero
// value is valid and means that the model/provider defaults apply.
type Options struct {
	Model            string       `json:"model,omitempty"`
	FrequencyPenalty *float64     `json:"frequency_penalty,omitempty"`
	MaxTokens        *int64       `json:"max_tokens,omitempty"`
	PresencePenalty  *float64     `json:"presence_penalty,omitempty"`
	Stop             []string     `json:"stop,omitempty"`
	Temperature      *float64     `json:"temperature,omitempty"`
	TopK             *int64       `json:"top_k,omitempty"`
	TopP             *float64     `json:"top_p,omitempty"`
	Extensions       metadata.Map `json:"extensions,omitzero"`
}

// NewOptions builds Options for the given model id.
func NewOptions(model string) (Options, error) {
	if model == "" {
		return Options{}, fmt.Errorf("chat.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	options := Options{Model: model}
	if err := options.Validate(); err != nil {
		return Options{}, fmt.Errorf("chat.NewOptions: %w", err)
	}
	return options, nil
}

// SetExtension encodes a provider-specific option under a namespace/name key.
func (o *Options) SetExtension(key string, value any) error {
	if o == nil {
		return fmt.Errorf("chat.Options.SetExtension: %w: nil receiver", ErrInvalidOptions)
	}
	if err := extension.Set(&o.Extensions, key, value); err != nil {
		return fmt.Errorf("chat.Options.SetExtension: %w: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns an independent copy of o.
func (o Options) Clone() Options {
	return Options{
		Model:            o.Model,
		FrequencyPenalty: ptr.Clone(o.FrequencyPenalty),
		MaxTokens:        ptr.Clone(o.MaxTokens),
		PresencePenalty:  ptr.Clone(o.PresencePenalty),
		Stop:             slices.Clone(o.Stop),
		Temperature:      ptr.Clone(o.Temperature),
		TopK:             ptr.Clone(o.TopK),
		TopP:             ptr.Clone(o.TopP),
		Extensions:       o.Extensions.Clone(),
	}
}

// Merged clones o and applies each override left-to-right.
func (o Options) Merged(overrides ...Options) (Options, error) {
	merged := o.Clone()
	for _, override := range overrides {
		if err := merged.applyOverride(override); err != nil {
			return Options{}, fmt.Errorf("chat.Options.Merged: %w: %w", ErrInvalidOptions, err)
		}
	}
	if err := merged.Validate(); err != nil {
		return Options{}, fmt.Errorf("chat.Options.Merged: %w", err)
	}
	return merged, nil
}

func (o *Options) applyOverride(src Options) error {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.FrequencyPenalty != nil {
		o.FrequencyPenalty = ptr.Clone(src.FrequencyPenalty)
	}
	if src.MaxTokens != nil {
		o.MaxTokens = ptr.Clone(src.MaxTokens)
	}
	if src.PresencePenalty != nil {
		o.PresencePenalty = ptr.Clone(src.PresencePenalty)
	}
	if src.Stop != nil {
		o.Stop = slices.Clone(src.Stop)
	}
	if src.Temperature != nil {
		o.Temperature = ptr.Clone(src.Temperature)
	}
	if src.TopK != nil {
		o.TopK = ptr.Clone(src.TopK)
	}
	if src.TopP != nil {
		o.TopP = ptr.Clone(src.TopP)
	}
	if len(src.Extensions) > 0 {
		if err := o.Extensions.Merge(src.Extensions); err != nil {
			return fmt.Errorf("merge extensions: %w", err)
		}
	}
	return nil
}

// Validate verifies explicitly supplied overrides. Options{} is valid.
func (o Options) Validate() error {
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model must not have surrounding whitespace", ErrInvalidOptions)
	}
	if err := validateFloat("frequency_penalty", o.FrequencyPenalty, -2, 2); err != nil {
		return err
	}
	if o.MaxTokens != nil && *o.MaxTokens <= 0 {
		return fmt.Errorf("%w: max_tokens must be greater than zero", ErrInvalidOptions)
	}
	if err := validateFloat("presence_penalty", o.PresencePenalty, -2, 2); err != nil {
		return err
	}
	for i, stop := range o.Stop {
		if stop == "" {
			return fmt.Errorf("%w: stop[%d] must not be empty", ErrInvalidOptions, i)
		}
	}
	if err := validateFloat("temperature", o.Temperature, 0, 2); err != nil {
		return err
	}
	if o.TopK != nil && *o.TopK <= 0 {
		return fmt.Errorf("%w: top_k must be greater than zero", ErrInvalidOptions)
	}
	if err := validateFloat("top_p", o.TopP, 0, 1); err != nil {
		return err
	}
	if err := extension.Validate(o.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidOptions, err)
	}
	return nil
}

func validateFloat(name string, value *float64, minValue, maxValue float64) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < minValue || *value > maxValue {
		return fmt.Errorf("%w: %s must be between %g and %g", ErrInvalidOptions, name, minValue, maxValue)
	}
	return nil
}

// MarshalJSON validates Options before writing its wire representation.
func (o Options) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	type wireOptions Options
	return json.Marshal(wireOptions(o))
}

// UnmarshalJSON decodes and validates Options before replacing the receiver.
func (o *Options) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidOptions)
	}
	type wireOptions Options
	var decoded wireOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidOptions, err)
	}
	candidate := Options(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*o = candidate
	return nil
}
