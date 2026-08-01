package rag

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
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

// CompressionTransformerConfig configures [NewCompressionTransformer].
type CompressionTransformerConfig struct {
	// ChatModel performs the compression. Required.
	ChatModel chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [compressionDefaultTemplate]. Custom templates must declare
	// {{.History}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

var _ Transformer = (*compressionTransformer)(nil)

type compressionTransformer struct {
	chatClient     *chatclient.Client
	promptTemplate *chatclient.Template
}

type compressionPromptVariables struct {
	History string
	Query   string
}

// NewCompressionTransformer returns a [Transformer] that collapses chat history
// plus a follow-up question into a single self-contained query. It reads chat
// history from the query value stored under [ChatHistoryValueKey].
func NewCompressionTransformer(cfg CompressionTransformerConfig) (Transformer, error) {
	if cfg.ChatModel == nil {
		return nil, errors.New("rag: compression transformer requires a chat model")
	}
	promptTemplate, err := resolvePromptTemplate(
		cfg.PromptTemplate,
		compressionDefaultTemplate,
		promptVariableHistory,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	client, err := chatclient.New(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &compressionTransformer{
		chatClient:     client,
		promptTemplate: promptTemplate,
	}, nil
}

// Transform asks the LLM for a self-contained version of the query.
// Returns a clone of the input with Text replaced by the LLM output.
func (c *compressionTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	history, err := c.extractHistory(query)
	if err != nil {
		return nil, err
	}

	compressed, err := callPrompt(ctx, c.chatClient, c.promptTemplate, compressionPromptVariables{
		History: history,
		Query:   query.Text(),
	})
	if err != nil {
		return nil, err
	}

	return query.WithText(compressed)
}

// extractHistory pulls the conversation messages out of the query value under
// [ChatHistoryValueKey] and renders them as one string.
// Returns "" when the slot is missing.
func (c *compressionTransformer) extractHistory(query *Query) (string, error) {
	messages, exists, err := LookupValue(query, chatHistoryValueKey)
	if err != nil {
		return "", fmt.Errorf("rag: read chat history: %w", err)
	}
	if !exists {
		return "", nil
	}
	return formatChatHistory(messages)
}
