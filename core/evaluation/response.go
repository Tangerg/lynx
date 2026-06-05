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
	if r == nil || r.Metadata == nil {
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

// parseScoredResponse extracts the first float in [0, 1] from an LLM
// reply and builds a [*Response]:
//
//   - Score: the parsed value, clamped exactly to the matched float
//     (out-of-range matches are skipped, not clamped — the parser tries
//     the next match, so "5 out of 10 → 0.5" still works if 0.5 appears).
//   - Pass:  Score >= threshold.
//   - Feedback: trimmed text after the score token, so the LLM's
//     reasoning surfaces without manual extraction.
//
// Returns an error if no number in [0, 1] is found — silent fallback
// to zero would hide LLM format failures.
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
