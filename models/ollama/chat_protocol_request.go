package ollama

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	ollamaapi "github.com/ollama/ollama/api"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

const (
	// RequestExtensionKey stores official Ollama /api/chat request fields that
	// have no provider-neutral Core equivalent. Core model, messages, tools,
	// streaming, and common options take precedence.
	RequestExtensionKey = "ollama/request"

	protocolGeneratedToolPrefix = "ollama/generated/"
)

func mapProtocolRequest(defaults corechat.Options, req *corechat.Request, stream bool) (*ollamaapi.ChatRequest, error) {
	options := defaults.Overlay(req.Options)
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("ollama: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("ollama: model is required in defaults or request options")
	}

	apiReq, err := decodeProtocolRequestExtension(req)
	if err != nil {
		return nil, err
	}
	apiReq.Model = options.Model
	apiReq.Stream = &stream
	apiReq.Messages, err = mapProtocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	apiReq.Tools, err = mapProtocolTools(req.Tools)
	if err != nil {
		return nil, err
	}
	if apiReq.Options == nil {
		apiReq.Options = make(map[string]any)
	}
	if options.FrequencyPenalty != nil {
		apiReq.Options["frequency_penalty"] = float32(*options.FrequencyPenalty)
	}
	if options.MaxTokens != nil {
		value, err := protocolInt("max_tokens", *options.MaxTokens)
		if err != nil {
			return nil, err
		}
		apiReq.Options["num_predict"] = value
	}
	if options.PresencePenalty != nil {
		apiReq.Options["presence_penalty"] = float32(*options.PresencePenalty)
	}
	if len(options.Stop) > 0 {
		apiReq.Options["stop"] = slices.Clone(options.Stop)
	}
	if options.Temperature != nil {
		apiReq.Options["temperature"] = float32(*options.Temperature)
	}
	if options.TopK != nil {
		value, err := protocolInt("top_k", *options.TopK)
		if err != nil {
			return nil, err
		}
		apiReq.Options["top_k"] = value
	}
	if options.TopP != nil {
		apiReq.Options["top_p"] = float32(*options.TopP)
	}
	if apiReq.TopLogprobs < 0 || apiReq.TopLogprobs > 20 {
		return nil, errors.New("ollama: top_logprobs must be between 0 and 20")
	}
	return apiReq, nil
}

func decodeProtocolRequestExtension(req *corechat.Request) (*ollamaapi.ChatRequest, error) {
	apiReq := new(ollamaapi.ChatRequest)
	raw, found := req.Extensions[RequestExtensionKey]
	if !found {
		return apiReq, nil
	}
	if err := json.Unmarshal(raw, apiReq); err != nil {
		return nil, fmt.Errorf("ollama: extension %q: %w", RequestExtensionKey, err)
	}
	return apiReq, nil
}

func protocolInt(name string, value int64) (int, error) {
	maxInt := int64(int(^uint(0) >> 1))
	if value > maxInt {
		return 0, fmt.Errorf("ollama: options.%s exceeds int", name)
	}
	return int(value), nil
}

func mapProtocolMessages(messages []corechat.Message) ([]ollamaapi.Message, error) {
	mapped := make([]ollamaapi.Message, 0, len(messages))
	for i := range messages {
		message := messages[i]
		switch message.Role {
		case corechat.RoleSystem:
			mapped = append(mapped, ollamaapi.Message{Role: "system", Content: message.Text()})
		case corechat.RoleUser:
			user, err := mapProtocolUserMessage(message)
			if err != nil {
				return nil, fmt.Errorf("ollama: messages[%d]: %w", i, err)
			}
			mapped = append(mapped, user)
		case corechat.RoleAssistant:
			assistant, err := mapProtocolAssistantMessage(message)
			if err != nil {
				return nil, fmt.Errorf("ollama: messages[%d]: %w", i, err)
			}
			mapped = append(mapped, assistant)
		case corechat.RoleTool:
			for j := range message.Parts {
				result := message.Parts[j].ToolResult
				id := result.ID
				if strings.HasPrefix(id, protocolGeneratedToolPrefix) {
					id = ""
				}
				mapped = append(mapped, ollamaapi.Message{
					Role:       "tool",
					Content:    result.Result,
					ToolName:   result.Name,
					ToolCallID: id,
				})
			}
		default:
			return nil, fmt.Errorf("ollama: messages[%d]: unsupported role %q", i, message.Role)
		}
	}
	return mapped, nil
}

func mapProtocolUserMessage(message corechat.Message) (ollamaapi.Message, error) {
	mapped := ollamaapi.Message{Role: "user"}
	var text strings.Builder
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			text.WriteString(part.Text)
		case corechat.PartMedia:
			image, err := mapProtocolImage(part.Media)
			if err != nil {
				return ollamaapi.Message{}, fmt.Errorf("parts[%d]: %w", i, err)
			}
			mapped.Images = append(mapped.Images, image)
		default:
			return ollamaapi.Message{}, fmt.Errorf("parts[%d]: unsupported user part %q", i, part.Kind)
		}
	}
	mapped.Content = text.String()
	return mapped, nil
}

func mapProtocolImage(value *media.Media) (ollamaapi.ImageData, error) {
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return nil, fmt.Errorf("media MIME %q is not an image", value.MIME)
	}
	if value.Source.Kind != media.SourceBytes {
		return nil, fmt.Errorf("image source %q is unsupported; Ollama requires bytes", value.Source.Kind)
	}
	data, err := value.Bytes()
	if err != nil {
		return nil, err
	}
	return ollamaapi.ImageData(data), nil
}

func mapProtocolAssistantMessage(message corechat.Message) (ollamaapi.Message, error) {
	mapped := ollamaapi.Message{Role: "assistant"}
	var text, reasoning strings.Builder
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			text.WriteString(part.Text)
		case corechat.PartReasoning:
			if len(part.Signature) > 0 {
				return ollamaapi.Message{}, fmt.Errorf("parts[%d]: reasoning signature is unsupported", i)
			}
			reasoning.WriteString(part.Text)
		case corechat.PartToolCall:
			arguments, err := mapProtocolToolArguments(part.ToolCall.Arguments)
			if err != nil {
				return ollamaapi.Message{}, fmt.Errorf("parts[%d].tool_call.arguments: %w", i, err)
			}
			id := part.ToolCall.ID
			if strings.HasPrefix(id, protocolGeneratedToolPrefix) {
				id = ""
			}
			mapped.ToolCalls = append(mapped.ToolCalls, ollamaapi.ToolCall{
				ID: id,
				Function: ollamaapi.ToolCallFunction{
					Index:     len(mapped.ToolCalls),
					Name:      part.ToolCall.Name,
					Arguments: arguments,
				},
			})
		case corechat.PartMedia:
			return ollamaapi.Message{}, fmt.Errorf("parts[%d]: assistant media is unsupported", i)
		default:
			return ollamaapi.Message{}, fmt.Errorf("parts[%d]: unsupported assistant part %q", i, part.Kind)
		}
	}
	mapped.Content = text.String()
	mapped.Thinking = reasoning.String()
	return mapped, nil
}

func mapProtocolToolArguments(arguments string) (ollamaapi.ToolCallFunctionArguments, error) {
	if arguments == "" {
		return ollamaapi.NewToolCallFunctionArguments(), nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &object); err != nil || object == nil {
		if err == nil {
			err = errors.New("must be a JSON object")
		}
		return ollamaapi.ToolCallFunctionArguments{}, err
	}
	var mapped ollamaapi.ToolCallFunctionArguments
	if err := json.Unmarshal([]byte(arguments), &mapped); err != nil {
		return ollamaapi.ToolCallFunctionArguments{}, err
	}
	return mapped, nil
}

func mapProtocolTools(definitions []corechat.ToolDefinition) (ollamaapi.Tools, error) {
	if len(definitions) == 0 {
		return nil, nil
	}
	mapped := make(ollamaapi.Tools, 0, len(definitions))
	for i := range definitions {
		var parameters ollamaapi.ToolFunctionParameters
		if err := json.Unmarshal(definitions[i].InputSchema, &parameters); err != nil {
			return nil, fmt.Errorf("ollama: tools[%d].input_schema: %w", i, err)
		}
		mapped = append(mapped, ollamaapi.Tool{
			Type: "function",
			Function: ollamaapi.ToolFunction{
				Name:        definitions[i].Name,
				Description: definitions[i].Description,
				Parameters:  parameters,
			},
		})
	}
	return mapped, nil
}
