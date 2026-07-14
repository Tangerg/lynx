package chatclient

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func prepareRequest(request *chat.Request, defaults chat.Options) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	prepared := cloneRequest(request)
	prepared.Options = mergeOptions(defaults, request.Options)
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

func cloneRequest(request *chat.Request) *chat.Request {
	clone := &chat.Request{
		Messages:   make([]chat.Message, len(request.Messages)),
		Tools:      make([]chat.ToolDefinition, len(request.Tools)),
		Options:    cloneOptions(request.Options),
		Extensions: request.Extensions.Clone(),
	}
	for i := range request.Messages {
		clone.Messages[i] = cloneMessage(request.Messages[i])
	}
	for i := range request.Tools {
		clone.Tools[i] = request.Tools[i]
		clone.Tools[i].InputSchema = append(json.RawMessage(nil), request.Tools[i].InputSchema...)
	}
	return clone
}

func cloneMessage(message chat.Message) chat.Message {
	clone := chat.Message{
		Role:     message.Role,
		Parts:    make([]chat.Part, len(message.Parts)),
		Metadata: message.Metadata.Clone(),
	}
	for i := range message.Parts {
		clone.Parts[i] = clonePart(message.Parts[i])
	}
	return clone
}

func clonePart(part chat.Part) chat.Part {
	clone := part
	clone.Signature = slices.Clone(part.Signature)
	if part.Media != nil {
		clone.Media = cloneMedia(part.Media)
	}
	if part.ToolCall != nil {
		value := *part.ToolCall
		clone.ToolCall = &value
	}
	if part.ToolResult != nil {
		value := *part.ToolResult
		clone.ToolResult = &value
	}
	return clone
}

func cloneMedia(value *media.Media) *media.Media {
	clone := *value
	clone.Source.Bytes = slices.Clone(value.Source.Bytes)
	clone.Metadata = value.Metadata.Clone()
	return &clone
}

func mergeOptions(defaults, overrides chat.Options) chat.Options {
	merged := cloneOptions(defaults)
	if overrides.Model != "" {
		merged.Model = overrides.Model
	}
	if overrides.FrequencyPenalty != nil {
		merged.FrequencyPenalty = clonePointer(overrides.FrequencyPenalty)
	}
	if overrides.MaxTokens != nil {
		merged.MaxTokens = clonePointer(overrides.MaxTokens)
	}
	if overrides.PresencePenalty != nil {
		merged.PresencePenalty = clonePointer(overrides.PresencePenalty)
	}
	if overrides.Stop != nil {
		merged.Stop = slices.Clone(overrides.Stop)
	}
	if overrides.Temperature != nil {
		merged.Temperature = clonePointer(overrides.Temperature)
	}
	if overrides.TopK != nil {
		merged.TopK = clonePointer(overrides.TopK)
	}
	if overrides.TopP != nil {
		merged.TopP = clonePointer(overrides.TopP)
	}
	return merged
}

func cloneOptions(options chat.Options) chat.Options {
	return chat.Options{
		Model:            options.Model,
		FrequencyPenalty: clonePointer(options.FrequencyPenalty),
		MaxTokens:        clonePointer(options.MaxTokens),
		PresencePenalty:  clonePointer(options.PresencePenalty),
		Stop:             slices.Clone(options.Stop),
		Temperature:      clonePointer(options.Temperature),
		TopK:             clonePointer(options.TopK),
		TopP:             clonePointer(options.TopP),
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
