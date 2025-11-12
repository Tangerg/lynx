package evaluation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
)

var _ Evaluator = (*FactCheckingEvaluator)(nil)

// FactCheckingEvaluatorConfig  holds the configuration for FactCheckingEvaluator.
type FactCheckingEvaluatorConfig struct {
	// ChatModel The language model used to perform fact-checking evaluations
	ChatModel chat.Model

	// PromptTemplate Custom template for fact-checking prompts (optional)
	// If not provided, a default template will be used
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

// FactCheckingEvaluator evaluates whether a generated claim is factually supported
// by the provided source documents using a language model.
//
// The evaluator uses a prompt-based approach where the LLM is asked to determine
// if a claim is supported by reference documents, responding with "YES" or "NO".
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
				WithVariable("Document", extractDocuments(req)).
				WithVariable("Claim", req.Generation),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	return buildResponse(text)
}
