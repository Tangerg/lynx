package evaluation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

var (
	ErrInvalidConfig  = errors.New("evaluation: invalid config")
	ErrInvalidRequest = errors.New("evaluation: invalid request")
	ErrInvalidResult  = errors.New("evaluation: invalid result")
	ErrNoScore        = errors.New("evaluation: no score in [0, 1]")
)

// DefaultPassThreshold is used when ModelConfig.Threshold is zero.
const DefaultPassThreshold = 0.5

// Request is the plain-data input shared by RAG evaluators. Context contains
// caller-selected source text; evaluation deliberately does not depend on a
// document storage or retrieval type.
type Request struct {
	Query   string   `json:"query,omitempty"`
	Answer  string   `json:"answer,omitempty"`
	Context []string `json:"context,omitempty"`
}

func (r Request) contextText() string {
	texts := make([]string, 0, len(r.Context))
	for _, text := range r.Context {
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

// Result is one normalized evaluation verdict.
type Result struct {
	Pass     bool         `json:"pass"`
	Score    float64      `json:"score"`
	Feedback string       `json:"feedback,omitempty"`
	Metadata metadata.Map `json:"metadata,omitempty"`
}

// Validate verifies the normalized score and JSON-safe metadata.
func (r Result) Validate() error {
	if math.IsNaN(r.Score) || math.IsInf(r.Score, 0) || r.Score < 0 || r.Score > 1 {
		return fmt.Errorf("%w: score must be between 0 and 1", ErrInvalidResult)
	}
	if err := r.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResult, err)
	}
	return nil
}

// Evaluator scores one generated answer.
type Evaluator interface {
	Evaluate(ctx context.Context, request Request) (Result, error)
}

// EvaluatorFunc adapts an ordinary function to Evaluator.
type EvaluatorFunc func(ctx context.Context, request Request) (Result, error)

// Evaluate invokes f.
func (f EvaluatorFunc) Evaluate(ctx context.Context, request Request) (Result, error) {
	return f(ctx, request)
}
