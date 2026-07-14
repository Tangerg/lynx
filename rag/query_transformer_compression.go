package rag

import (
	"context"
	"errors"

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

func (c *CompressionTransformerConfig) validate() error {
	if c.ChatModel == nil {
		return errors.New("rag.CompressionTransformerConfig: ChatModel is required")
	}
	if c.PromptTemplate != nil {
		return c.PromptTemplate.Require("History", "Query")
	}
	return nil
}

func (c *CompressionTransformerConfig) applyDefaults() error {
	if c.PromptTemplate == nil {
		var err error
		c.PromptTemplate, err = chatclient.ParseTemplate(compressionDefaultTemplate)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ Transformer = (*compressionTransformer)(nil)

type compressionTransformer struct {
	chatClient     *chatclient.Client
	promptTemplate *chatclient.Template
}

// NewCompressionTransformer returns a [Transformer] that collapses chat history
// plus a follow-up question into a single self-contained query. It reads chat
// history from [Query.Extra] under [ChatHistoryKey].
func NewCompressionTransformer(cfg CompressionTransformerConfig) (Transformer, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chatclient.New(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &compressionTransformer{
		chatClient:     client,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

// Transform asks the LLM for a self-contained version of the query.
// Returns a clone of the input with Text replaced by the LLM output;
// when the LLM returns empty text the original Text is preserved.
func (c *compressionTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	history := c.extractHistory(query)

	compressed, err := callTemplate(ctx, c.chatClient, c.promptTemplate, map[string]any{
		"History": history,
		"Query":   query.Text,
	})
	if err != nil {
		return nil, err
	}

	return query.withModelText(compressed), nil
}

// extractHistory pulls the conversation messages out of the query's
// Extra map under [ChatHistoryKey] and renders them as one string.
// Returns "" when the slot is missing or holds the wrong type.
func (c *compressionTransformer) extractHistory(query *Query) string {
	value, exists := query.Get(ChatHistoryKey)
	if !exists {
		return ""
	}

	messages, ok := value.([]chat.Message)
	if !ok {
		return ""
	}
	return formatChatHistory(messages)
}
