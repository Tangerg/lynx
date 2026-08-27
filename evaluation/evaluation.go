package evaluation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

var (
	ErrInvalidConfig = errors.New("evaluation: invalid config")
	ErrInvalidScore  = errors.New("evaluation: invalid score")
	ErrInvalidSample = errors.New("evaluation: invalid sample")
	ErrInvalidReport = errors.New("evaluation: invalid report")
)

// DefaultThreshold is used when [ModelConfig.Threshold] is nil.
const DefaultThreshold Score = 0.5

// Score is a normalized evaluation score in the closed interval [0, 1].
type Score float64

// NewScore validates value and returns a normalized score.
func NewScore(value float64) (Score, error) {
	score := Score(value)
	if err := score.Validate(); err != nil {
		return 0, err
	}
	return score, nil
}

// Float64 returns the score as its wire representation.
func (s Score) Float64() float64 { return float64(s) }

// Validate verifies that the score is finite and normalized.
func (s Score) Validate() error {
	value := s.Float64()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%w: must be between 0 and 1", ErrInvalidScore)
	}
	return nil
}

// Passes reports whether the score meets threshold. Invalid values never pass.
func (s Score) Passes(threshold Score) bool {
	return s.Validate() == nil && threshold.Validate() == nil && s >= threshold
}

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

// Clone returns an independent copy of the sample.
func (t TextSample) Clone() TextSample {
	t.Context = slices.Clone(t.Context)
	return t
}

// ContextText returns non-blank context entries joined in caller order.
func (t TextSample) ContextText() string {
	texts := make([]string, 0, len(t.Context))
	for _, text := range t.Context {
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

// Report is one normalized evaluation verdict. Details contains the owned
// child reports of a composite evaluation instead of flattening them into
// convention-based metadata keys.
type Report struct {
	Passed   bool         `json:"passed"`
	Score    Score        `json:"score"`
	Feedback string       `json:"feedback,omitzero"`
	Metadata metadata.Map `json:"metadata,omitzero"`
	Details  []Report     `json:"details,omitzero"`
}

// Clone returns an independent report, including metadata and child reports.
func (r Report) Clone() Report {
	r.Metadata = r.Metadata.Clone()
	r.Details = slices.Clone(r.Details)
	for i := range r.Details {
		r.Details[i] = r.Details[i].Clone()
	}
	return r
}

// Validate verifies the normalized score, JSON-safe metadata, and nested
// reports.
func (r Report) Validate() error {
	if err := r.Score.Validate(); err != nil {
		return fmt.Errorf("%w: score: %w", ErrInvalidReport, err)
	}
	if err := r.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidReport, err)
	}
	for i, detail := range r.Details {
		if err := detail.Validate(); err != nil {
			return fmt.Errorf("%w: details[%d]: %w", ErrInvalidReport, i, err)
		}
	}
	return nil
}

// Evaluator evaluates one subject and returns a normalized report.
type Evaluator[T any] interface {
	Evaluate(context.Context, T) (Report, error)
}

// EvaluatorFunc adapts an ordinary function to Evaluator.
type EvaluatorFunc[T any] func(context.Context, T) (Report, error)

// Evaluate invokes e.
func (e EvaluatorFunc[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	return e(ctx, subject)
}
