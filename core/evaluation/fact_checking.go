package evaluation

import (
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// factCheckingDefaultTemplate asks the LLM for a continuous
// fact-support score in [0, 1] — 1.0 = fully supported by the
// document, 0.0 = unsupported / contradicted. {{.Document}} and
// {{.Claim}} are filled at evaluation time. The score must be on the
// first non-empty line; reasoning follows after.
const factCheckingDefaultTemplate = `Evaluate how well the claim is supported by the provided document.

Reply with a single number between 0.0 and 1.0 on the first line, where:
  1.0 = the claim is fully supported by the document
  0.5 = the claim is partially supported
  0.0 = the claim is not supported or contradicted
Then on the next line, briefly explain your reasoning.

Document:
{{.Document}}

Claim:
{{.Claim}}

Score:`

var _ Evaluator = (*FactCheckingEvaluator)(nil)

// FactCheckingEvaluatorConfig configures a [FactCheckingEvaluator].
// ChatModel is required; PromptTemplate falls back to a scored default;
// Threshold defaults to [DefaultPassThreshold].
type FactCheckingEvaluatorConfig struct {
	// ChatModel scores the claim against the document. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [factCheckingDefaultTemplate]. Custom templates must declare
	// {{.Document}} and {{.Claim}} and instruct the model to emit a
	// number in [0, 1].
	PromptTemplate *chat.PromptTemplate

	// Threshold is the score boundary at which [Response.Pass] flips
	// from false to true. Zero falls back to [DefaultPassThreshold].
	Threshold float64
}

// ApplyDefaults fills PromptTemplate when nil.
func (c *FactCheckingEvaluatorConfig) ApplyDefaults() {
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(factCheckingDefaultTemplate)
	}
}

// Validate returns an error when required fields are missing or the
// template lacks the expected variables. Pure check — pair with
// [FactCheckingEvaluatorConfig.ApplyDefaults].
func (c FactCheckingEvaluatorConfig) Validate() error {
	if c.ChatModel == nil {
		return errors.New("evaluation.FactCheckingEvaluatorConfig: ChatModel is required")
	}
	if c.PromptTemplate == nil {
		return nil
	}
	return c.PromptTemplate.RequireVariables("Document", "Claim")
}

// FactCheckingEvaluator scores whether an AI-generated claim is
// supported by the supplied source documents — the standard
// fact-verification check for RAG outputs.
//
// Verdicts:
//   - Score: the LLM's continuous judgment, parsed from its reply, in [0, 1].
//   - Pass:  true when Score >= the configured Threshold (default 0.5).
//   - Feedback: the model's reasoning, taken from text after the score token.
type FactCheckingEvaluator struct {
	*llmEvaluator
}

// NewFactCheckingEvaluator builds a [FactCheckingEvaluator] from
// config. Returns an error when the configuration fails validation or
// the chat client can't be constructed.
func NewFactCheckingEvaluator(config FactCheckingEvaluatorConfig) (*FactCheckingEvaluator, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	config.ApplyDefaults()
	base, err := newLLMEvaluator(
		config.ChatModel,
		config.PromptTemplate,
		func(req *Request) map[string]any {
			return map[string]any{
				"Document": extractDocuments(req),
				"Claim":    req.Generation,
			}
		},
		config.Threshold,
	)
	if err != nil {
		return nil, err
	}
	return &FactCheckingEvaluator{llmEvaluator: base}, nil
}
