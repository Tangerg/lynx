package evaluation

import (
	"fmt"
	"strings"
)

const relevancePrompt = `Evaluate how relevant and grounded the answer is for the query using the provided context.

Reply with a number between 0.0 and 1.0 on the first line, then briefly explain your reasoning.

Query:
{{.Query}}

Answer:
{{.Answer}}

Context:
{{.Context}}

Score:`

// RelevanceEvaluator scores whether an answer addresses its query and is
// grounded in source context.
type RelevanceEvaluator struct {
	*modelEvaluator
}

// NewRelevanceEvaluator constructs a relevance evaluator.
func NewRelevanceEvaluator(config ModelConfig) (*RelevanceEvaluator, error) {
	evaluator, err := newModelEvaluator(config, relevancePrompt, validateRelevanceRequest)
	if err != nil {
		return nil, err
	}
	return &RelevanceEvaluator{modelEvaluator: evaluator}, nil
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
