package openai

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"mime"
	"slices"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

func (c *Chat) buildRequest(req *corechat.Request, stream bool) (*openaisdk.ChatCompletionNewParams, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("openai: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}

	params := openaisdk.ChatCompletionNewParams{}
	if !c.dialect.DisableRawRequestExtension {
		extensionKey := protocolRequestExtensionKey(c.dialect.Provider)
		fields, err := decodeRequestFields(req.Options.Extensions, extensionKey,
			"model", "messages", "tools", "frequency_penalty", "max_tokens",
			"max_completion_tokens", "presence_penalty", "reasoning_effort", "response_format", "stop", "temperature", "top_p",
		)
		if err != nil {
			return nil, err
		}
		if _, exists := fields["n"]; exists {
			return nil, fmt.Errorf("openai: extension %q field %q is unsupported; Core Chat produces one output", extensionKey, "n")
		}
		params.SetExtraFields(fields)
	}

	options, err := c.defaults.Resolve(req.Options)
	if err != nil {
		return nil, fmt.Errorf("openai: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("openai: model is required in defaults or request options")
	}
	if options.TopK != nil {
		return nil, errors.New("openai: options.top_k is not supported by Chat Completions")
	}
	params.Model = openaisdk.ChatModel(options.Model)
	if options.FrequencyPenalty != nil {
		params.FrequencyPenalty = openaisdk.Float(*options.FrequencyPenalty)
	}
	if options.MaxTokens != nil {
		switch c.dialect.TokenLimitField {
		case TokenLimitMaxTokens:
			params.MaxTokens = openaisdk.Int(*options.MaxTokens)
		case TokenLimitMaxCompletionTokens:
			params.MaxCompletionTokens = openaisdk.Int(*options.MaxTokens)
		default:
			return nil, errors.New("openai: invalid max token field configuration")
		}
	}
	if options.PresencePenalty != nil {
		params.PresencePenalty = openaisdk.Float(*options.PresencePenalty)
	}
	reasoningEffort, err := mapReasoningEffort(options.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	params.ReasoningEffort = reasoningEffort
	if len(options.Stop) > 0 {
		params.Stop.OfStringArray = slices.Clone(options.Stop)
	}
	if options.Temperature != nil {
		params.Temperature = openaisdk.Float(*options.Temperature)
	}
	if options.TopP != nil {
		params.TopP = openaisdk.Float(*options.TopP)
	}

	params.Messages, err = mapRequestMessages(req.Messages, c.dialect.Provider)
	if err != nil {
		return nil, err
	}
	params.Tools, err = mapToolDefinitions(req.Tools)
	if err != nil {
		return nil, err
	}
	if err := applyChatOutputFormat(options.OutputFormat, &params, c.dialect); err != nil {
		return nil, err
	}
	if c.dialect.request != nil {
		if err := c.dialect.request.PrepareRequest(req, &params); err != nil {
			return nil, fmt.Errorf("openai: request dialect: %w", err)
		}
	}
	if c.dialect.PrepareRequest != nil {
		compatible := &CompatibleRequest{
			model:       string(params.Model),
			stream:      stream,
			extraFields: maps.Clone(params.ExtraFields()),
		}
		if params.Temperature.Valid() {
			compatible.temperature = &params.Temperature.Value
		}
		if err := c.dialect.PrepareRequest(req, compatible); err != nil {
			return nil, fmt.Errorf("openai: compatible request dialect: %w", err)
		}
		params.SetExtraFields(compatible.extraFields)
	}
	return &params, nil
}

func mapToolDefinitions(definitions []corechat.ToolDefinition) ([]openaisdk.ChatCompletionToolUnionParam, error) {
	tools := make([]openaisdk.ChatCompletionToolUnionParam, 0, len(definitions))
	for i := range definitions {
		var schema map[string]any
		if err := json.Unmarshal(definitions[i].InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("openai: tools[%d].input_schema: %w", i, err)
		}
		tools = append(tools, openaisdk.ChatCompletionToolUnionParam{
			OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
				Function: openaisdk.FunctionDefinitionParam{
					Name:        definitions[i].Name,
					Description: openaisdk.String(definitions[i].Description),
					Parameters:  schema,
				},
			},
		})
	}
	return tools, nil
}

func mapRequestMessages(messages []corechat.Message, provider string) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	result := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for i := range messages {
		mapped, err := mapRequestMessage(messages[i], provider)
		if err != nil {
			return nil, fmt.Errorf("openai: messages[%d]: %w", i, err)
		}
		result = append(result, mapped...)
	}
	return result, nil
}

func mapRequestMessage(message corechat.Message, provider string) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	switch message.Role {
	case corechat.RoleSystem:
		return []openaisdk.ChatCompletionMessageParamUnion{openaisdk.SystemMessage(message.Text())}, nil
	case corechat.RoleUser:
		mapped, err := mapUserMessage(message)
		return []openaisdk.ChatCompletionMessageParamUnion{mapped}, err
	case corechat.RoleAssistant:
		mapped, err := mapAssistantMessage(message, provider)
		return []openaisdk.ChatCompletionMessageParamUnion{mapped}, err
	case corechat.RoleTool:
		mapped := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(message.Parts))
		for i := range message.Parts {
			result := message.Parts[i].ToolResult
			if result == nil {
				return nil, fmt.Errorf("parts[%d]: missing tool result", i)
			}
			mapped = append(mapped, openaisdk.ToolMessage(result.Result, result.ID))
		}
		return mapped, nil
	default:
		return nil, fmt.Errorf("unsupported role %q", message.Role)
	}
}

func mapUserMessage(message corechat.Message) (openaisdk.ChatCompletionMessageParamUnion, error) {
	if len(message.Parts) == 1 && message.Parts[0].Kind == corechat.PartText {
		return openaisdk.UserMessage(message.Parts[0].Text), nil
	}

	parts := make([]openaisdk.ChatCompletionContentPartUnionParam, 0, len(message.Parts))
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			parts = append(parts, openaisdk.ChatCompletionContentPartUnionParam{
				OfText: &openaisdk.ChatCompletionContentPartTextParam{Text: part.Text},
			})
		case corechat.PartMedia:
			mapped, err := mapUserMedia(part.Media)
			if err != nil {
				return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: %w", i, err)
			}
			parts = append(parts, mapped)
		default:
			return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: unsupported user part %q", i, part.Kind)
		}
	}
	return openaisdk.UserMessage(parts), nil
}

func mapUserMedia(value *media.Media) (openaisdk.ChatCompletionContentPartUnionParam, error) {
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil {
		return openaisdk.ChatCompletionContentPartUnionParam{}, fmt.Errorf("media MIME %q: %w", value.MIME, err)
	}
	major, subtype, _ := strings.Cut(mediaType, "/")

	switch major {
	case "image":
		location, locationErr := mediaLocation(value)
		if locationErr != nil {
			return openaisdk.ChatCompletionContentPartUnionParam{}, fmt.Errorf("image source: %w", locationErr)
		}
		return openaisdk.ChatCompletionContentPartUnionParam{
			OfImageURL: &openaisdk.ChatCompletionContentPartImageParam{
				ImageURL: openaisdk.ChatCompletionContentPartImageImageURLParam{URL: location},
			},
		}, nil
	case "audio":
		if value.Source.Kind != media.SourceBytes {
			return openaisdk.ChatCompletionContentPartUnionParam{}, fmt.Errorf("audio requires bytes, got %q", value.Source.Kind)
		}
		if subtype != "wav" && subtype != "mp3" {
			return openaisdk.ChatCompletionContentPartUnionParam{}, fmt.Errorf("audio MIME subtype %q is unsupported", subtype)
		}
		data, bytesErr := value.Bytes()
		if bytesErr != nil {
			return openaisdk.ChatCompletionContentPartUnionParam{}, bytesErr
		}
		return openaisdk.ChatCompletionContentPartUnionParam{
			OfInputAudio: &openaisdk.ChatCompletionContentPartInputAudioParam{
				InputAudio: openaisdk.ChatCompletionContentPartInputAudioInputAudioParam{
					Data:   base64.StdEncoding.EncodeToString(data),
					Format: subtype,
				},
			},
		}, nil
	default:
		file := openaisdk.ChatCompletionContentPartFileFileParam{Filename: openaisdk.String(value.Name)}
		switch value.Source.Kind {
		case media.SourceReference:
			ref, refErr := value.Reference()
			if refErr != nil {
				return openaisdk.ChatCompletionContentPartUnionParam{}, refErr
			}
			file.FileID = openaisdk.String(ref)
		case media.SourceBytes, media.SourceURI:
			location, locationErr := mediaLocation(value)
			if locationErr != nil {
				return openaisdk.ChatCompletionContentPartUnionParam{}, locationErr
			}
			file.FileData = openaisdk.String(location)
		default:
			return openaisdk.ChatCompletionContentPartUnionParam{}, fmt.Errorf("unsupported file source %q", value.Source.Kind)
		}
		return openaisdk.ChatCompletionContentPartUnionParam{
			OfFile: &openaisdk.ChatCompletionContentPartFileParam{File: file},
		}, nil
	}
}

func mediaLocation(value *media.Media) (string, error) {
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
		return "", fmt.Errorf("source %q cannot be represented as a URL", value.Source.Kind)
	}
}

func mapAssistantMessage(message corechat.Message, provider string) (openaisdk.ChatCompletionMessageParamUnion, error) {
	mapped := openaisdk.AssistantMessage(message.Text())
	assistant := mapped.OfAssistant
	var audioID string
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			// Text was flattened in order by Message.Text above.
		case corechat.PartToolCall:
			assistant.ToolCalls = append(assistant.ToolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
					ID: part.ToolCall.ID,
					Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      part.ToolCall.Name,
						Arguments: part.ToolCall.Arguments,
					},
				},
			})
		case corechat.PartReasoning:
			// Reasoning is provider-owned replay state. The selected dialect
			// decides whether and how it is represented on the wire.
		case corechat.PartMedia:
			if audioID != "" {
				return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: Chat Completions supports at most one assistant audio part", i)
			}
			if part.Media.Source.Kind != media.SourceReference || part.Media.Source.Ref == "" {
				return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: assistant audio replay requires a provider reference", i)
			}
			audioID = part.Media.Source.Ref
		default:
			return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: unsupported assistant part %q", i, part.Kind)
		}
	}

	if audioID != "" {
		assistant.Audio.ID = audioID
	}
	refusalKey := protocolRefusalExtensionKey(provider)
	refusal, found, err := message.Metadata.Decode[string](refusalKey)
	if err != nil {
		return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("metadata %q: %w", refusalKey, err)
	}
	if found {
		assistant.Refusal = openaisdk.String(refusal)
	}
	return mapped, nil
}
