package evaluation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// factCheckingDefaultTemplate asks the LLM whether the supplied claim
// is factually supported by the document. The variables {{.Document}}
// and {{.Claim}} are filled at evaluation time.
const factCheckingDefaultTemplate = `Evaluate whether or not the following claim is supported by the provided document.
Respond with "YES" if the claim is supported, or "NO" if it is not.

Document:
{{.Document}}

Claim:
{{.Claim}}`

var _ Evaluator = (*FactCheckingEvaluator)(nil)

// FactCheckingEvaluatorConfig configures a [FactCheckingEvaluator].
// ChatModel is required; PromptTemplate falls back to a default that
// asks YES/NO.
type FactCheckingEvaluatorConfig struct {
	// ChatModel scores the claim against the document. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [factCheckingDefaultTemplate]. Custom templates must declare
	// {{.Document}} and {{.Claim}}.
	PromptTemplate *chat.PromptTemplate
}

// validate fills the default prompt template and returns an error when
// required fields are missing or the template lacks the expected
// variables.
func (c *FactCheckingEvaluatorConfig) validate() error {
	if c.ChatModel == nil {
		return errors.New("evaluation.FactCheckingEvaluatorConfig: ChatModel is required")
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate().WithTemplate(factCheckingDefaultTemplate)
	}
	return c.PromptTemplate.RequireVariables("Document", "Claim")
}

// FactCheckingEvaluator scores whether an AI-generated claim is
// supported by the supplied source documents — the standard
// fact-verification check for RAG outputs.
//
// Verdicts:
//   - Pass: true when the LLM answers "YES",
//   - Score: 1.0 for YES, 0.0 otherwise.
type FactCheckingEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

// NewFactCheckingEvaluator builds a [FactCheckingEvaluator] from
// config. Returns an error when the configuration fails validation or
// the chat client can't be constructed.
func NewFactCheckingEvaluator(config FactCheckingEvaluatorConfig) (*FactCheckingEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	client, err := chat.NewClientWithModel(config.ChatModel)
	if err != nil {
		return nil, err
	}
	return &FactCheckingEvaluator{
		chatClient:     client,
		promptTemplate: config.PromptTemplate,
	}, nil
}

// Evaluate asks the LLM whether req.Generation is supported by the
// retrieved documents, then returns a YES/NO-shaped [*Response].
func (f *FactCheckingEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("evaluation.FactCheckingEvaluator.Evaluate: request must not be nil")
	}

	text, _, err := f.chatClient.
		ChatWithPromptTemplate(
			f.promptTemplate.Clone().
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
