package evaluation

import (
	"errors"
	"fmt"
	"strings"
)

// Response is one evaluator's verdict — a pass/fail flag, a numerical
// score (typically 0..1), human-readable feedback, and free-form
// metadata.
type Response struct {
	// Pass is the binary verdict: did the response satisfy this
	// evaluator's criteria.
	Pass bool `json:"pass"`

	// Score is the continuous score, conventionally in [0, 1].
	Score float64 `json:"score"`

	// Feedback is human-readable rationale.
	Feedback string `json:"feedback,omitempty"`

	// Metadata carries arbitrary per-evaluation extras.
	Metadata map[string]any `json:"metadata,omitzero"`
}

// ensureMetadata lazily allocates Metadata. Used by [Response.Set]
// only — Get must not mutate state.
func (r *Response) ensureMetadata() {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
}

// Get returns the Metadata value for key plus an existence flag.
// Safe to call concurrently with other Get calls; concurrent with
// Set is not.
func (r *Response) Get(key string) (any, bool) {
	if r.Metadata == nil {
		return nil, false
	}
	v, ok := r.Metadata[key]
	return v, ok
}

// Set stores value under key in Metadata.
func (r *Response) Set(key string, value any) {
	r.ensureMetadata()
	r.Metadata[key] = value
}

// parseYesNoResponse maps an LLM YES/NO answer into a [*Response]:
// pass=true + score=1.0 for "YES" (case-insensitive, whitespace-
// trimmed), pass=false + score=0.0 otherwise.
func parseYesNoResponse(text string) (*Response, error) {
	pass := strings.EqualFold(strings.TrimSpace(text), "YES")
	score := 0.0
	if pass {
		score = 1.0
	}
	return &Response{Pass: pass, Score: score}, nil
}

// mergeResponses combines multiple sub-evaluations into one verdict —
// AND on Pass, average on Score, concatenated and prefixed Feedback,
// namespace-prefixed Metadata keys plus "total_evaluations" /
// "passed_count" summary fields.
func mergeResponses(responses []*Response) (*Response, error) {
	if len(responses) == 0 {
		return nil, errors.New("evaluation.mergeResponses: at least one response is required")
	}
	if len(responses) == 1 {
		return responses[0], nil
	}

	merged := &Response{
		Pass:     true,
		Metadata: make(map[string]any),
	}

	var (
		feedbacks   []string
		totalScore  float64
		passedCount int
	)

	for i, resp := range responses {
		merged.Pass = merged.Pass && resp.Pass
		totalScore += resp.Score
		if resp.Pass {
			passedCount++
		}

		if resp.Feedback != "" {
			feedbacks = append(feedbacks, fmt.Sprintf("[Evaluation %d] %s", i+1, resp.Feedback))
		}

		for key, value := range resp.Metadata {
			merged.Metadata[fmt.Sprintf("eval_%d_%s", i+1, key)] = value
		}
	}

	merged.Score = totalScore / float64(len(responses))
	merged.Feedback = strings.Join(feedbacks, "\n\n")
	merged.Set("total_evaluations", len(responses))
	merged.Set("passed_count", passedCount)

	return merged, nil
}
