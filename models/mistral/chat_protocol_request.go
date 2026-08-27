package mistral

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

func (c *Chat) buildRequest(request *corechat.Request, stream bool) (*chatCompletionRequest, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("mistral: nil Chat")
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("mistral: request: %w", err)
	}
	extension, found, err := request.Options.Extensions.Decode[ChatRequestOptions](RequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("mistral: extension %q: %w", RequestExtensionKey, err)
	}
	if found {
		fields, _, decodeErr := request.Options.Extensions.Decode[map[string]json.RawMessage](RequestExtensionKey)
		if decodeErr != nil {
			return nil, fmt.Errorf("mistral: extension %q: %w", RequestExtensionKey, decodeErr)
		}
		if _, exists := fields["response_format"]; exists {
			return nil, fmt.Errorf("mistral: extension %q field %q is owned by options.output_format", RequestExtensionKey, "response_format")
		}
		if validateErr := extension.Validate(); validateErr != nil {
			return nil, fmt.Errorf("mistral: extension %q: %w", RequestExtensionKey, validateErr)
		}
	}
	options, err := c.defaults.Resolve(request.Options)
	if err != nil {
		return nil, fmt.Errorf("mistral: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("mistral: model is required in defaults or request options")
	}
	if options.TopK != nil {
		return nil, errors.New("mistral: options.top_k is not supported")
	}
	if options.Temperature != nil && (*options.Temperature < 0 || *options.Temperature > 1.5) {
		return nil, fmt.Errorf("mistral: options.temperature must be between 0 and 1.5, got %v", *options.Temperature)
	}
	messages, err := mapChatRequestMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	tools, err := mapChatTools(request.Tools)
	if err != nil {
		return nil, err
	}
	responseFormat, err := mapMistralOutputFormat(options.OutputFormat)
	if err != nil {
		return nil, err
	}
	return &chatCompletionRequest{
		Model:              options.Model,
		Messages:           messages,
		Temperature:        options.Temperature,
		TopP:               options.TopP,
		MaxTokens:          options.MaxTokens,
		Stream:             stream,
		Stop:               slices.Clone(options.Stop),
		PresencePenalty:    options.PresencePenalty,
		FrequencyPenalty:   options.FrequencyPenalty,
		Tools:              tools,
		ResponseFormat:     responseFormat,
		ChatRequestOptions: extension,
	}, nil
}

func mapMistralOutputFormat(format *corechat.OutputFormat) (json.RawMessage, error) {
	if format == nil {
		return nil, nil
	}
	var value any
	switch format.Type {
	case corechat.OutputFormatText:
		value = map[string]string{"type": "text"}
	case corechat.OutputFormatJSON:
		value = map[string]string{"type": "json_object"}
	case corechat.OutputFormatJSONSchema:
		schema, err := format.SchemaAs[map[string]any]()
		if err != nil {
			return nil, fmt.Errorf("mistral: output schema: %w", err)
		}
		definition := map[string]any{"name": format.Name, "schema": schema, "strict": true}
		if format.Description != "" {
			definition["description"] = format.Description
		}
		value = map[string]any{"type": "json_schema", "json_schema": definition}
	default:
		return nil, fmt.Errorf("mistral: unsupported output format %q", format.Type)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("mistral: encode output format: %w", err)
	}
	return encoded, nil
}

func mapChatRequestMessages(messages []corechat.Message) ([]chatMessage, error) {
	result := make([]chatMessage, 0, len(messages))
	for messageIndex := range messages {
		message := messages[messageIndex]
		switch message.Role {
		case corechat.RoleSystem:
			result = append(result, chatMessage{Role: "system", Content: message.Text()})
		case corechat.RoleUser:
			content, err := mapMistralUserContent(message.Parts)
			if err != nil {
				return nil, fmt.Errorf("mistral: messages[%d]: %w", messageIndex, err)
			}
			result = append(result, chatMessage{Role: "user", Content: content})
		case corechat.RoleAssistant:
			mapped, err := mapMistralAssistantMessage(message.Parts)
			if err != nil {
				return nil, fmt.Errorf("mistral: messages[%d]: %w", messageIndex, err)
			}
			result = append(result, mapped)
		case corechat.RoleTool:
			for partIndex := range message.Parts {
				toolResult := message.Parts[partIndex].ToolResult
				if toolResult == nil {
					return nil, fmt.Errorf("mistral: messages[%d].parts[%d]: missing tool result", messageIndex, partIndex)
				}
				result = append(result, chatMessage{
					Role:       "tool",
					Content:    toolResult.Result,
					ToolCallID: toolResult.ID,
					Name:       toolResult.Name,
				})
			}
		default:
			return nil, fmt.Errorf("mistral: messages[%d]: unsupported role %q", messageIndex, message.Role)
		}
	}
	return result, nil
}

func mapMistralUserContent(parts []corechat.Part) (any, error) {
	if len(parts) == 1 && parts[0].Kind == corechat.PartText {
		return parts[0].Text, nil
	}
	content := make([]any, 0, len(parts))
	for partIndex := range parts {
		part := parts[partIndex]
		switch part.Kind {
		case corechat.PartText:
			content = append(content, textChunk{Type: "text", Text: part.Text})
		case corechat.PartMedia:
			chunk, err := mapMistralMedia(part.Media)
			if err != nil {
				return nil, fmt.Errorf("parts[%d]: %w", partIndex, err)
			}
			content = append(content, chunk)
		default:
			return nil, fmt.Errorf("parts[%d]: unsupported user part %q", partIndex, part.Kind)
		}
	}
	return content, nil
}

func mapMistralMedia(value *media.Media) (any, error) {
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil {
		return nil, fmt.Errorf("media MIME %q: %w", value.MIME, err)
	}
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		uri, err := mistralMediaURI(value)
		if err != nil {
			return nil, err
		}
		return imageURLChunk{Type: "image_url", ImageURL: imageURLValue(uri)}, nil
	case mediaType == "application/pdf":
		if value.Source.Kind != media.SourceURI {
			return nil, errors.New("PDF input requires a URL source")
		}
		uri, err := value.URI()
		if err != nil {
			return nil, err
		}
		return documentURLChunk{Type: "document_url", DocumentURL: uri, DocumentName: value.Name}, nil
	case strings.HasPrefix(mediaType, "audio/"):
		if value.Source.Kind != media.SourceBytes {
			return nil, errors.New("audio input requires a byte source")
		}
		data, err := value.Bytes()
		if err != nil {
			return nil, err
		}
		return audioChunk{Type: "input_audio", InputAudio: base64.StdEncoding.EncodeToString(data)}, nil
	default:
		return nil, fmt.Errorf("media MIME %q is unsupported", mediaType)
	}
}

func mistralMediaURI(value *media.Media) (string, error) {
	switch value.Source.Kind {
	case media.SourceURI:
		return value.URI()
	case media.SourceBytes:
		data, err := value.Bytes()
		if err != nil {
			return "", err
		}
		return "data:" + value.MIME + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	default:
		return "", fmt.Errorf("media source %q is unsupported", value.Source.Kind)
	}
}

func mapMistralAssistantMessage(parts []corechat.Part) (chatMessage, error) {
	message := chatMessage{Role: "assistant"}
	content := make([]any, 0, len(parts))
	for partIndex := range parts {
		part := parts[partIndex]
		switch part.Kind {
		case corechat.PartText:
			content = append(content, textChunk{Type: "text", Text: part.Text})
		case corechat.PartReasoning:
			frames, framed, err := decodeThinkingFrames(part.Signature)
			if err != nil {
				return chatMessage{}, fmt.Errorf("parts[%d].reasoning signature: %w", partIndex, err)
			}
			if framed {
				chunk, err := coalesceThinkingFrames(frames)
				if err != nil {
					return chatMessage{}, fmt.Errorf("parts[%d].reasoning signature: %w", partIndex, err)
				}
				content = append(content, chunk)
				continue
			}
			if part.Text != "" {
				content = append(content, thinkChunk{
					Type:     "thinking",
					Thinking: []textChunk{{Type: "text", Text: part.Text}},
					Closed:   true,
				})
			}
		case corechat.PartToolCall:
			arguments := json.RawMessage(part.ToolCall.Arguments)
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			if !json.Valid(arguments) {
				return chatMessage{}, fmt.Errorf("parts[%d].tool_call.arguments contains invalid JSON", partIndex)
			}
			message.ToolCalls = append(message.ToolCalls, chatToolCall{
				ID:   part.ToolCall.ID,
				Type: "function",
				Function: chatFunctionCall{
					Name:      part.ToolCall.Name,
					Arguments: arguments,
				},
			})
		default:
			return chatMessage{}, fmt.Errorf("parts[%d]: unsupported assistant part %q", partIndex, part.Kind)
		}
	}
	if len(content) == 1 {
		if text, ok := content[0].(textChunk); ok {
			message.Content = text.Text
		} else {
			message.Content = content
		}
	} else if len(content) > 0 {
		message.Content = content
	}
	return message, nil
}

func mapChatTools(definitions []corechat.ToolDefinition) ([]chatTool, error) {
	tools := make([]chatTool, 0, len(definitions))
	for index := range definitions {
		var parameters map[string]any
		if err := json.Unmarshal(definitions[index].InputSchema, &parameters); err != nil {
			return nil, fmt.Errorf("mistral: tools[%d].input_schema: %w", index, err)
		}
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        definitions[index].Name,
				Description: definitions[index].Description,
				Parameters:  parameters,
			},
		})
	}
	return tools, nil
}
