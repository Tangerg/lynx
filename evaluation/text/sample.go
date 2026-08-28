// Package text evaluates generated text against its originating input and
// caller-selected evidence.
package text

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrInvalidSample = errors.New("evaluation/text: invalid sample")

const evidenceSeparator = "\n"

// Sample contains the generated-text facts used by text metrics. Individual
// evaluators require only the fields their metric consumes.
type Sample struct {
	Input   string   `json:"input,omitzero"`
	Output  string   `json:"output,omitzero"`
	Context []string `json:"context,omitzero"`
}

func NewSample(input, output string, context []string) Sample {
	return Sample{Input: input, Output: output, Context: slices.Clone(context)}
}

func (sample Sample) Clone() Sample {
	sample.Context = slices.Clone(sample.Context)
	return sample
}

// ContextText drops blank evidence entries before joining them in caller order.
func (sample Sample) ContextText() string {
	texts := make([]string, 0, len(sample.Context))
	for _, text := range sample.Context {
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, evidenceSeparator)
}

func (sample Sample) validateAnswerRelevance() error {
	if strings.TrimSpace(sample.Input) == "" {
		return fmt.Errorf("%w: input is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	return nil
}

func (sample Sample) validateGroundedness() error {
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	if sample.ContextText() == "" {
		return fmt.Errorf("%w: context is required", ErrInvalidSample)
	}
	return nil
}
