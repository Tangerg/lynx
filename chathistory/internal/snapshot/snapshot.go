// Package snapshot deep-copies caller-owned chat protocol values at history
// ownership boundaries.
package snapshot

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// Messages validates and deep-copies messages.
func Messages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for i := range messages {
		if err := messages[i].Validate(); err != nil {
			return nil, fmt.Errorf("chathistory: messages[%d]: %w", i, err)
		}
		cloned[i] = Message(messages[i])
	}
	return cloned, nil
}

// Request validates and deep-copies a request.
func Request(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	cloned := &chat.Request{
		Messages:   make([]chat.Message, len(request.Messages)),
		Tools:      make([]chat.ToolDefinition, len(request.Tools)),
		Options:    options(request.Options),
		Extensions: request.Extensions.Clone(),
	}
	for i := range request.Messages {
		cloned.Messages[i] = Message(request.Messages[i])
	}
	for i := range request.Tools {
		cloned.Tools[i] = request.Tools[i]
		cloned.Tools[i].InputSchema = slices.Clone(request.Tools[i].InputSchema)
	}
	return cloned, nil
}

// Message deep-copies a message already known to be valid.
func Message(message chat.Message) chat.Message {
	cloned := chat.Message{
		Role:     message.Role,
		Parts:    make([]chat.Part, len(message.Parts)),
		Metadata: message.Metadata.Clone(),
	}
	for i := range message.Parts {
		cloned.Parts[i] = part(message.Parts[i])
	}
	return cloned
}

func part(value chat.Part) chat.Part {
	cloned := value
	cloned.Signature = slices.Clone(value.Signature)
	if value.Media != nil {
		cloned.Media = mediaValue(value.Media)
	}
	if value.ToolCall != nil {
		toolCall := *value.ToolCall
		cloned.ToolCall = &toolCall
	}
	if value.ToolResult != nil {
		toolResult := *value.ToolResult
		cloned.ToolResult = &toolResult
	}
	return cloned
}

func mediaValue(value *media.Media) *media.Media {
	cloned := *value
	cloned.Source.Bytes = slices.Clone(value.Source.Bytes)
	cloned.Metadata = value.Metadata.Clone()
	return &cloned
}

func options(value chat.Options) chat.Options {
	return chat.Options{
		Model:            value.Model,
		FrequencyPenalty: pointer(value.FrequencyPenalty),
		MaxTokens:        pointer(value.MaxTokens),
		PresencePenalty:  pointer(value.PresencePenalty),
		Stop:             slices.Clone(value.Stop),
		Temperature:      pointer(value.Temperature),
		TopK:             pointer(value.TopK),
		TopP:             pointer(value.TopP),
	}
}

func pointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
