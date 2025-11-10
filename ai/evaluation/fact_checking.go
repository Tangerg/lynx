package evaluation

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ Evaluator = (*FactCheckingEvaluator)(nil)

type FactCheckingEvaluatorConfig struct {
	ChatModel      chat.Model
	PromptTemplate *chat.PromptTemplate
}

func (c *FactCheckingEvaluatorConfig) validate() error {
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
				`Evaluate whether or not the following claim is supported by the provided document.
				Respond with "YES" if the claim is supported, or "NO" if it is not.

				Document:
				{{.Document}}

				Claim:
				{{.Claim}}`,
			)
	}
	return c.PromptTemplate.RequireVariables("Document", "Claim")
}

type FactCheckingEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

func NewFactCheckingEvaluator(config *FactCheckingEvaluatorConfig) (*FactCheckingEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	chatClient, err := chat.NewClientWithModel(config.ChatModel)
	if err != nil {
		return nil, err
	}
	return &FactCheckingEvaluator{
		chatClient:     chatClient,
		promptTemplate: config.PromptTemplate,
	}, nil
}

func (f *FactCheckingEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	text, _, err := f.
		chatClient.
		ChatPromptTemplate(
			f.promptTemplate.
				Clone().
				WithVariable("Document", getSupportingData(req)).
				WithVariable("Claim", req.Generation),
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
