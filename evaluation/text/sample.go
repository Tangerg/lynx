// Package text evaluates generated text without imposing one shared sample on
// metrics with different semantic inputs.
package text

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrInvalidSample = errors.New("evaluation/text: invalid sample")

const evidenceSeparator = "\n"

type AnswerRelevanceSample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

func (sample AnswerRelevanceSample) Validate() error {
	if strings.TrimSpace(sample.Input) == "" {
		return fmt.Errorf("%w: input is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	return nil
}

type GroundednessSample struct {
	Output   string   `json:"output"`
	Evidence []string `json:"evidence"`
}

func (sample GroundednessSample) Clone() GroundednessSample {
	sample.Evidence = slices.Clone(sample.Evidence)
	return sample
}

func (sample GroundednessSample) EvidenceText() string {
	texts := make([]string, 0, len(sample.Evidence))
	for _, text := range sample.Evidence {
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, evidenceSeparator)
}

func (sample GroundednessSample) Validate() error {
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	if sample.EvidenceText() == "" {
		return fmt.Errorf("%w: evidence is required", ErrInvalidSample)
	}
	return nil
}

type CorrectnessSample struct {
	Input     string `json:"input"`
	Output    string `json:"output"`
	Reference string `json:"reference"`
}

func (sample CorrectnessSample) Validate() error {
	if strings.TrimSpace(sample.Input) == "" {
		return fmt.Errorf("%w: input is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Reference) == "" {
		return fmt.Errorf("%w: reference is required", ErrInvalidSample)
	}
	return nil
}
