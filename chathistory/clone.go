package chathistory

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func cloneMessages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for i := range messages {
		if err := messages[i].Validate(); err != nil {
			return nil, fmt.Errorf("chathistory: messages[%d]: %w", i, err)
		}
		cloned[i] = cloneMessage(messages[i])
	}
	return cloned, nil
}

func cloneMessage(message chat.Message) chat.Message {
	cloned := chat.Message{
		Role:     message.Role,
		Parts:    make([]chat.Part, len(message.Parts)),
		Metadata: message.Metadata.Clone(),
	}
	for i := range message.Parts {
		cloned.Parts[i] = clonePart(message.Parts[i])
	}
	return cloned
}

func clonePart(part chat.Part) chat.Part {
	cloned := part
	cloned.Signature = slices.Clone(part.Signature)
	if part.Media != nil {
		cloned.Media = cloneMedia(part.Media)
	}
	if part.ToolCall != nil {
		value := *part.ToolCall
		cloned.ToolCall = &value
	}
	if part.ToolResult != nil {
		value := *part.ToolResult
		cloned.ToolResult = &value
	}
	return cloned
}

func cloneMedia(value *media.Media) *media.Media {
	cloned := *value
	cloned.Source.Bytes = slices.Clone(value.Source.Bytes)
	cloned.Metadata = value.Metadata.Clone()
	return &cloned
}
