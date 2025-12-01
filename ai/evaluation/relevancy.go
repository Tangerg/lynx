package evaluation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ Evaluator = (*RelevancyEvaluator)(nil)

// RelevancyEvaluatorConfig holds the configuration for RelevancyEvaluator.
type RelevancyEvaluatorConfig struct {
	// ChatModel is the language model used to perform relevancy evaluation.
	// Required. Must be provided to analyze the relationship between
	// the response and the context.
	ChatModel chat.Model

	// PromptTemplate defines how the relevancy evaluation prompt is structured.
	// Optional. If not provided, a default template will be used that asks
	// the model to determine if the response aligns with the provided context
	// by answering "YES" or "NO".
	PromptTemplate *chat.PromptTemplate
}

func (c *RelevancyEvaluatorConfig) validate() error {
	if c == nil {
		return errors.New("nil Config")
	}
	if c.ChatModel == nil {
		return errors.New("nil ChatModel")
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`Your task is to evaluate if the response for the query
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

				Answer:`,
			)
	}
	return c.PromptTemplate.RequireVariables("Query", "Response", "Context")
}

var _ Evaluator = (*RelevancyEvaluator)(nil)

// RelevancyEvaluator evaluates whether a generated response is relevant to
// and consistent with the provided context information.
//
// This evaluator is useful for:
//   - Verifying that responses are grounded in the provided context
//   - Detecting hallucinations or fabricated information
//   - Ensuring answers stay within the scope of available knowledge
//   - Quality assurance in RAG (Retrieval-Augmented Generation) systems
//
// The evaluator returns:
//   - Pass: true if the response aligns with context, false otherwise
//   - Score: 1.0 for relevant responses, 0.0 for irrelevant ones
type RelevancyEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

func NewRelevancyEvaluatorConfig(config *RelevancyEvaluatorConfig) (*RelevancyEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	chatClient, err := chat.NewClientWithModel(config.ChatModel)
	if err != nil {
		return nil, err
	}
	return &RelevancyEvaluator{
		chatClient:     chatClient,
		promptTemplate: config.PromptTemplate,
	}, nil

}

func (r *RelevancyEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}

	text, _, err := r.
		chatClient.
		ChatWithPromptTemplate(
			r.promptTemplate.
				Clone().
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
