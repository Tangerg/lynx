package evaluation

import (
	"context"
	"fmt"
	"strings"
)

const relevancePrompt = `Evaluate how relevant and grounded the answer is for the query using the provided context.

Score relevance from 0.0 to 1.0 and provide concise feedback.

Query:
{{.Query}}

Answer:
{{.Answer}}

Context:
{{.Context}}

Evaluation:`

// RelevanceEvaluator scores whether an answer addresses its query and is
// grounded in source context.
type RelevanceEvaluator struct {
	evaluator *modelEvaluator
}

// NewRelevanceEvaluator constructs a relevance evaluator.
func NewRelevanceEvaluator(config ModelConfig) (*RelevanceEvaluator, error) {
	evaluator, err := newModelEvaluator(
		config,
		relevancePrompt,
		validateRelevanceRequest,
		"Query",
		"Answer",
		"Context",
	)
	if err != nil {
		return nil, err
	}
	return &RelevanceEvaluator{evaluator: evaluator}, nil
}

// Evaluate scores request for relevance and grounding.
func (e *RelevanceEvaluator) Evaluate(ctx context.Context, request Request) (Result, error) {
	return e.evaluator.Evaluate(ctx, request)
}

func validateRelevanceRequest(request Request) error {
	if strings.TrimSpace(request.Query) == "" {
		return fmt.Errorf("%w: query is required", ErrInvalidRequest)
	}
	if err := validateFactRequest(request); err != nil {
		return err
	}
	return nil
}
