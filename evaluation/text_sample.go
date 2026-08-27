package evaluation

import (
	"fmt"
	"slices"
	"strings"
)

// TextSample is the common input for generated-text evaluation. Input is the
// originating instruction or question, Output is the generated text, and
// Context contains caller-selected evidence. Individual evaluators validate
// only the fields their metric needs.
type TextSample struct {
	Input   string   `json:"input,omitzero"`
	Output  string   `json:"output,omitzero"`
	Context []string `json:"context,omitzero"`
}

// NewTextSample snapshots context so later caller mutation cannot change an
// in-flight evaluation input.
func NewTextSample(input, output string, context []string) TextSample {
	return TextSample{Input: input, Output: output, Context: slices.Clone(context)}
}

func (sample TextSample) Clone() TextSample {
	sample.Context = slices.Clone(sample.Context)
	return sample
}

// ContextText drops blank evidence entries before joining them in caller order.
func (sample TextSample) ContextText() string {
	texts := make([]string, 0, len(sample.Context))
	for _, text := range sample.Context {
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func (sample TextSample) validateAnswerRelevance() error {
	if strings.TrimSpace(sample.Input) == "" {
		return fmt.Errorf("%w: input is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	return nil
}

func (sample TextSample) validateGroundedness() error {
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	if sample.ContextText() == "" {
		return fmt.Errorf("%w: context is required", ErrInvalidSample)
	}
	return nil
}
