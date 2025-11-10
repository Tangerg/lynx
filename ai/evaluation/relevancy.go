package evaluation

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ Evaluator = (*RelevancyEvaluator)(nil)

type RelevancyEvaluatorConfig struct {
	ChatModel      chat.Model
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
		ChatPromptTemplate(
			r.promptTemplate.
				Clone().
				WithVariable("Query", req.Prompt).
				WithVariable("Response", req.Generation).
				WithVariable("Context", getSupportingData(req)),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}
	return &Response{
		Pass: strings.EqualFold(text, "YES"),
	}, nil
}
