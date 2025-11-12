package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/model/chat"
)

// CompressionQueryTransformerConfig holds the configuration for CompressionQueryTransformer.
// It requires a chat model and optionally accepts a custom prompt template.
type CompressionQueryTransformerConfig struct {
	// ChatModel is the language model used for query compression.
	// Required.
	ChatModel chat.Model

	// PromptTemplate defines how the compression prompt is structured.
	// Optional. If not provided, a default template will be used that combines
	// conversation history and follow-up query into a standalone query.
	PromptTemplate *chat.PromptTemplate
}

func (c *CompressionQueryTransformerConfig) validate() error {
	if c == nil {
		return errors.New("compression transformer config cannot be nil")
	}

	if c.ChatModel == nil {
		return errors.New("compression transformer config: chat model is required")
	}

	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`Given the following conversation history and a follow-up query, your task is to synthesize
a concise, standalone query that incorporates the context from the history.
Ensure the standalone query is clear, specific, and maintains the user's intent.

Conversation history:
{{.History}}

Follow-up query:
{{.Query}}

Standalone query:`,
			)
	}

	return c.PromptTemplate.RequireVariables("History", "Query")
}

var _ QueryTransformer = (*CompressionQueryTransformer)(nil)

// CompressionQueryTransformer uses a large language model to compress a conversation
// history and a follow-up query into a standalone query that captures the essence
// of the conversation.
//
// This transformer is useful when the conversation history is long and the follow-up
// query is related to the conversation context. It helps to:
//   - Reduce token usage by condensing the context
//   - Improve retrieval relevance by creating self-contained queries
//   - Maintain the semantic intent of the original query
type CompressionQueryTransformer struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
}

func NewCompressionQueryTransformer(cfg *CompressionQueryTransformerConfig) (*CompressionQueryTransformer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClientWithModel(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &CompressionQueryTransformer{
		chatClient:     client,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

func (c *CompressionQueryTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	conversationHistory := c.extractConversationHistory(query)

	compressedText, _, err := c.
		chatClient.
		ChatPromptTemplate(
			c.promptTemplate.
				Clone().
				WithVariable("History", conversationHistory).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clonedQuery := query.Clone()
	if compressedText != "" {
		clonedQuery.Text = compressedText
	}

	return clonedQuery, nil
}

func (c *CompressionQueryTransformer) extractConversationHistory(query *Query) string {
	historyValue, exists := query.Get(ChatHistoryKey)
	if !exists {
		return ""
	}

	messages, ok := historyValue.([]chat.Message)
	if !ok {
		return ""
	}

	return strings.Join(chat.MessagesToStrings(messages), "\n\n")
}
