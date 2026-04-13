package evaluation

import (
	"errors"
	"fmt"
	"strings"
)

// Response represents the result of an evaluation.
// It includes a Pass/fail status, numerical score, textual feedback,
// and additional metadata about the evaluation.
type Response struct {
	// Pass Whether the response passed the evaluation criteria
	Pass bool

	// Score Numerical score for the evaluation (typically 0.0-1.0)
	Score float64

	// Feedback Textual feedback explaining the evaluation results
	Feedback string

	// Metadata Additional evaluation metadata as key-value pairs
	Metadata map[string]any
}

func (r *Response) ensureMetadata() {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
}

func (r *Response) Get(key string) (any, bool) {
	r.ensureMetadata()

	v, ok := r.Metadata[key]
	return v, ok
}

func (r *Response) Set(key string, value any) {
	r.ensureMetadata()

	r.Metadata[key] = value
}

func buildResponse(text string) (*Response, error) {
	yes := strings.EqualFold(text, "YES")
	score := float64(0)
	if yes {
		score = 1
	}
	return &Response{
		Pass:  yes,
		Score: score,
	}, nil
}

func mergeResponses(responses []*Response) (*Response, error) {
	if len(responses) == 0 {
		return nil, errors.New("empty responses")
	}

	if len(responses) == 1 {
		return responses[0], nil
	}

	merged := &Response{
		Pass:     true,
		Score:    0.0,
		Metadata: make(map[string]any),
	}

	var feedbacks []string
	var totalScore float64
	var passedCount int

	for i, resp := range responses {
		merged.Pass = merged.Pass && resp.Pass
		totalScore += resp.Score
		if resp.Pass {
			passedCount++
		}

		if resp.Feedback != "" {
			feedbacks = append(feedbacks,
				fmt.Sprintf("[Evaluation %d] %s", i+1, resp.Feedback))
		}

		for key, value := range resp.Metadata {
			prefixedKey := fmt.Sprintf("eval_%d_%s", i+1, key)
			merged.Metadata[prefixedKey] = value
		}
	}

	merged.Score = totalScore / float64(len(responses))

	merged.Feedback = strings.Join(feedbacks, "\n\n")

	merged.Set("total_evaluations", len(responses))
	merged.Set("passed_count", passedCount)

	return merged, nil
}
