package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrInvalidOptions = errors.New("chat: invalid options")

// Options contains provider-neutral per-request generation overrides. Its zero
// value is valid and means that the model/provider defaults apply.
type Options struct {
	Model            string   `json:"model,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	MaxTokens        *int64   `json:"max_tokens,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	Stop             []string `json:"stop,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopK             *int64   `json:"top_k,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
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
		return fmt.Errorf("chat: decode options: %w", err)
	}
	candidate := Options(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*o = candidate
	return nil
}
