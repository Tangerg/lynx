package evaluation

import (
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// relevancyDefaultTemplate asks the model for a continuous relevance
// score in [0, 1] — 1.0 = fully grounded in the context, 0.0 = not
// supported at all. The variables {{.Query}}, {{.Response}},
// {{.Context}} are filled at evaluation time. The score sits on the
// first non-empty line; everything after it is surfaced as feedback.
const relevancyDefaultTemplate = `Your task is to evaluate how well the response for the query
is grounded in the context information provided.

Reply with a single number between 0.0 and 1.0 on the first line, where:
  1.0 = the response is fully supported by the context
  0.5 = the response is partially supported
  0.0 = the response is not supported at all
Then on the next line, briefly explain your reasoning.

Query:
{{.Query}}

Response:
{{.Response}}

Context:
{{.Context}}

Score:`

var _ Evaluator = (*RelevancyEvaluator)(nil)

// RelevancyEvaluatorConfig configures a [RelevancyEvaluator]. ChatModel
// is required; PromptTemplate falls back to a scored default;
// Threshold defaults to [DefaultPassThreshold].
type RelevancyEvaluatorConfig struct {
	// ChatModel scores the response against the context. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [relevancyDefaultTemplate]. Custom templates must declare the
	// variables {{.Query}}, {{.Response}}, {{.Context}} and instruct
	// the model to emit a number in [0, 1].
	PromptTemplate *chat.PromptTemplate

	// Threshold is the score boundary at which [Response.Pass] flips
	// from false to true. Zero falls back to [DefaultPassThreshold].
	Threshold float64
}

// ApplyDefaults fills PromptTemplate when nil.
func (c *RelevancyEvaluatorConfig) ApplyDefaults() {
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(relevancyDefaultTemplate)
	}
}

// Validate returns an error when required fields are missing or the
// template lacks the expected variables. Pure check — pair with
// [RelevancyEvaluatorConfig.ApplyDefaults].
func (c RelevancyEvaluatorConfig) Validate() error {
	if c.ChatModel == nil {
		return errors.New("evaluation.RelevancyEvaluatorConfig: ChatModel is required")
	}
	if c.PromptTemplate == nil {
		// Default would be applied; nothing to check.
		return nil
	}
	return c.PromptTemplate.RequireVariables("Query", "Response", "Context")
}

// RelevancyEvaluator scores whether an AI response is grounded in its
// retrieved context — the standard hallucination check for RAG
// pipelines.
//
// Verdicts:
//   - Score: the LLM's continuous judgment, parsed from its reply, in [0, 1].
//   - Pass:  true when Score >= the configured Threshold (default 0.5).
//   - Feedback: the model's reasoning, taken from text after the score token.
type RelevancyEvaluator struct {
	*llmEvaluator
}

// NewRelevancyEvaluator builds a [RelevancyEvaluator] from config.
func NewRelevancyEvaluator(config RelevancyEvaluatorConfig) (*RelevancyEvaluator, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	base, err := newLLMEvaluator(
		config.ChatModel,
		config.PromptTemplate,
		func(req *Request) map[string]any {
			return map[string]any{
				"Query":    req.Prompt,
				"Response": req.Generation,
				"Context":  extractDocuments(req),
			}
		},
		config.Threshold,
	)
	if err != nil {
		return nil, err
	}
	return &RelevancyEvaluator{llmEvaluator: base}, nil
}
