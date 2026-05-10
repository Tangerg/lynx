package evaluation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// relevancyDefaultTemplate asks the model to answer YES/NO on whether
// the response aligns with the supplied context. The variables
// {{.Query}}, {{.Response}}, {{.Context}} are filled at evaluation
// time.
const relevancyDefaultTemplate = `Your task is to evaluate if the response for the query
is in line with the context information provided.

You have two options to answer. Either "YES" or "NO".

Answer "YES", if the response for the query
is in line with context information otherwise "NO".

Query:
{{.Query}}

Response:
{{.Response}}

Context:
{{.Context}}

Answer:`

var _ Evaluator = (*RelevancyEvaluator)(nil)

// RelevancyEvaluatorConfig configures a [RelevancyEvaluator]. ChatModel
// is required; PromptTemplate falls back to a default that asks YES/NO.
type RelevancyEvaluatorConfig struct {
	// ChatModel scores the response against the context. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [relevancyDefaultTemplate]. Custom templates must declare the
	// variables {{.Query}}, {{.Response}}, {{.Context}}.
	PromptTemplate *chat.PromptTemplate
}

// validate fills the default prompt template and returns an error when
// required fields are missing or the template lacks the expected
// variables.
func (c *RelevancyEvaluatorConfig) validate() error {
	if c == nil {
		return errors.New("evaluation.RelevancyEvaluatorConfig: config must not be nil")
	}
	if c.ChatModel == nil {
		return errors.New("evaluation.RelevancyEvaluatorConfig: ChatModel is required")
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate().WithTemplate(relevancyDefaultTemplate)
	}
	return c.PromptTemplate.RequireVariables("Query", "Response", "Context")
}

// RelevancyEvaluator scores whether an AI response is grounded in its
// retrieved context — the standard hallucination check for RAG
// pipelines.
//
// Verdicts:
//   - Pass: true when the LLM answers "YES",
//   - Score: 1.0 for YES, 0.0 otherwise.
type RelevancyEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

// NewRelevancyEvaluator builds a [RelevancyEvaluator] from config.
func NewRelevancyEvaluator(config *RelevancyEvaluatorConfig) (*RelevancyEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	client, err := chat.NewClientWithModel(config.ChatModel)
	if err != nil {
		return nil, err
	}
	return &RelevancyEvaluator{
		chatClient:     client,
		promptTemplate: config.PromptTemplate,
	}, nil
}

// Evaluate asks the LLM whether req.Generation aligns with the
// retrieved context, then returns a YES/NO-shaped [*Response].
func (r *RelevancyEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("evaluation.RelevancyEvaluator.Evaluate: request must not be nil")
	}

	text, _, err := r.chatClient.
		ChatWithPromptTemplate(
			r.promptTemplate.Clone().
				WithVariable("Query", req.Prompt).
				WithVariable("Response", req.Generation).
				WithVariable("Context", extractDocuments(req)),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}
	return buildResponse(text)
}
