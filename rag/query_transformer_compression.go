package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chatclient"
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
	// Model performs the compression. Required.
	Model chat.Model

	// PromptTemplate is the LLM prompt. Defaults to
	// [compressionDefaultTemplate]. Custom templates must declare
	// {{.History}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

var _ Transformer = (*compressionTransformer)(nil)

type compressionTransformer struct {
	prompt modelPrompt
}

type compressionPromptVariables struct {
	History string
	Query   string
}

// NewCompressionTransformer returns a [Transformer] that collapses chat history
// plus a follow-up question into a single self-contained query. It reads chat
// history from the query value stored under [ChatHistoryValueKey].
func NewCompressionTransformer(cfg CompressionTransformerConfig) (Transformer, error) {
	prompt, err := newModelPrompt(
		cfg.Model,
		cfg.PromptTemplate,
		compressionDefaultTemplate,
		promptVariableHistory,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &compressionTransformer{prompt: prompt}, nil
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

	compressed, err := c.prompt.call(ctx, compressionPromptVariables{
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
	messages, exists, err := query.Value(chatHistoryValueKey)
	if err != nil {
		return "", fmt.Errorf("rag: read chat history: %w", err)
	}
	if !exists {
		return "", nil
	}
	return c.formatHistory(messages)
}

func (c *compressionTransformer) formatHistory(messages []chat.Message) (string, error) {
	var output strings.Builder
	for messageIndex := range messages {
		if err := messages[messageIndex].Validate(); err != nil {
			return "", fmt.Errorf("rag: format chat history message %d: %w", messageIndex, err)
		}
		if output.Len() != 0 {
			output.WriteString("\n\n")
		}
		fmt.Fprintf(&output, "%s: ", messages[messageIndex].Role)
		for partIndex := range messages[messageIndex].Parts {
			if partIndex != 0 {
				output.WriteString(" ")
			}
			part := messages[messageIndex].Parts[partIndex]
			switch part.Kind {
			case chat.PartText:
				output.WriteString(part.Text)
			case chat.PartMedia:
				fmt.Fprintf(&output, "[media %s]", part.Media.MIME)
			case chat.PartReasoning:
				output.WriteString("[reasoning omitted]")
			case chat.PartToolCall:
				fmt.Fprintf(&output, "[tool call %s %s]", part.ToolCall.Name, part.ToolCall.Arguments)
			case chat.PartToolResult:
				fmt.Fprintf(&output, "[tool result %s %s]", part.ToolResult.Name, part.ToolResult.Result)
			}
		}
	}
	return output.String(), nil
}
