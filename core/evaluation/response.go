package evaluation

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DefaultPassThreshold is the score boundary at which an evaluator
// flips its [Response.Pass] verdict from false to true. Evaluators
// expose [Threshold] in their config to override.
const DefaultPassThreshold = 0.5

// scoreNumberPattern matches a non-negative number with an optional
// decimal part — `0`, `1`, `0.5`, `.85`, `1.00`. Used by
// [parseScoredResponse] to find the first numeric token an LLM emitted
// regardless of surrounding noise ("SCORE: 0.85", "0.9 — solid", ...).
var scoreNumberPattern = regexp.MustCompile(`\d*\.\d+|\d+`)

// Response is one evaluator's verdict — a pass/fail flag, a numerical
// score (typically 0..1), human-readable feedback, and free-form
// metadata.
type Response struct {
	Pass bool `json:"pass"`

	// Score is the continuous score, conventionally in [0, 1].
	Score float64 `json:"score"`

	Feedback string `json:"feedback,omitempty"`
	Metadata map[string]any `json:"metadata,omitzero"`
}

func (r *Response) ensureMetadata() {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
}

func (r *Response) Get(key string) (any, bool) {
	if r == nil || r.Metadata == nil {
		return nil, false
	}
	v, ok := r.Metadata[key]
	return v, ok
}

func (r *Response) Set(key string, value any) {
	r.ensureMetadata()
	r.Metadata[key] = value
}

func parseScoredResponse(text string, threshold float64) (*Response, error) {
	for _, span := range scoreNumberPattern.FindAllStringIndex(text, -1) {
		token := text[span[0]:span[1]]
		score, err := strconv.ParseFloat(token, 64)
		if err != nil || score < 0 || score > 1 {
			continue
		}
		return &Response{
			Pass:     score >= threshold,
			Score:    score,
			Feedback: strings.TrimSpace(text[span[1]:]),
		}, nil
	}
	return nil, fmt.Errorf("evaluation: no score in [0, 1] found in response: %q", text)
}

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
