package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// compressionDefaultTemplate asks the LLM to fold a chat history plus a
// follow-up question into one self-contained query. {{.History}} and
// {{.Query}} are filled at transform time.
const compressionDefaultTemplate = `Given the following conversation history and a follow-up query, your task is to synthesize
a concise, standalone query that incorporates the context from the history.
Ensure the standalone query is clear, specific, and maintains the user's intent.

Conversation history:
{{.History}}

Follow-up query:
{{.Query}}

Standalone query:`

// CompressionTransformerConfig configures a
// [CompressionTransformer].
type CompressionTransformerConfig struct {
	// ChatModel performs the compression. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [compressionDefaultTemplate]. Custom templates must declare
	// {{.History}} and {{.Query}}.
	PromptTemplate *chat.PromptTemplate
}

// validate fills the default prompt template and rejects invalid
// configs.
func (c *CompressionTransformerConfig) validate() error {
	if c == nil {
		return errors.New("rag.CompressionTransformerConfig: config must not be nil")
	}
	if c.ChatModel == nil {
		return errors.New("rag.CompressionTransformerConfig: ChatModel is required")
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(compressionDefaultTemplate)
	}
	return c.PromptTemplate.RequireVariables("History", "Query")
}

var _ QueryTransformer = (*CompressionTransformer)(nil)

// CompressionTransformer collapses a chat history plus a follow-up
// query into a single self-contained query. Reach for it when the
// conversation context is long and a downstream retriever needs to
// understand the question without re-reading the full transcript.
//
// The transformer reads chat history from [Query.Extra] under
// [ChatHistoryKey] — populated by [NewPipelineMiddleware] when the
// pipeline runs as chat middleware.
type CompressionTransformer struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

// NewCompressionTransformer builds a
// [CompressionTransformer]. Returns an error when the
// configuration fails validation or the chat client cannot be
// constructed.
func NewCompressionTransformer(cfg *CompressionTransformerConfig) (*CompressionTransformer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClient(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &CompressionTransformer{
		chatClient:     client,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

// Transform asks the LLM for a self-contained version of the query.
// Returns a clone of the input with Text replaced by the LLM output;
// when the LLM returns empty text the original Text is preserved.
func (c *CompressionTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	history := c.extractHistory(query)

	compressed, _, err := c.chatClient.
		ChatWithPromptTemplate(
			c.promptTemplate.Clone().
				WithVariable("History", history).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clone := query.Clone()
	if compressed != "" {
		clone.Text = compressed
	}
	return clone, nil
}

// extractHistory pulls the conversation messages out of the query's
// Extra map under [ChatHistoryKey] and renders them as one string.
// Returns "" when the slot is missing or holds the wrong type.
func (c *CompressionTransformer) extractHistory(query *Query) string {
	value, exists := query.Get(ChatHistoryKey)
	if !exists {
		return ""
	}

	messages, ok := value.([]chat.Message)
	if !ok {
		return ""
	}
	return strings.Join(chat.MessagesToStrings(messages), "\n\n")
}
