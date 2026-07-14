package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	protocolRequestExtensionKey    = "anthropic/request"
	protocolRedactedReasoningKey   = "anthropic/redacted_reasoning"
	protocolNativeStopReasonKey    = "anthropic/native_stop_reason"
	protocolStopSequenceKey        = "anthropic/stop_sequence"
	protocolUsageKey               = "anthropic/usage"
	protocolRedactedReasoningDelta = "anthropic/redacted_reasoning_delta"
)

func mapProtocolRequest(defaults corechat.Options, req *corechat.Request) (*anthropicsdk.MessageNewParams, error) {
	params, found, err := metadata.Decode[anthropicsdk.MessageNewParams](req.Extensions, protocolRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("anthropic: extension %q: %w", protocolRequestExtensionKey, err)
	}
	if !found {
		params = anthropicsdk.MessageNewParams{}
	}

	options := mergeProtocolOptions(defaults, req.Options)
	if options.Model == "" {
		return nil, errors.New("anthropic: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil {
		return nil, errors.New("anthropic: options.frequency_penalty is not supported")
	}
	if options.PresencePenalty != nil {
		return nil, errors.New("anthropic: options.presence_penalty is not supported")
	}
	params.Model = options.Model
	if options.MaxTokens != nil {
		params.MaxTokens = *options.MaxTokens
	} else if params.MaxTokens == 0 {
		params.MaxTokens = protocolDefaultMaxTokens
	}
	if options.Temperature != nil {
		params.Temperature = param.NewOpt(*options.Temperature)
	}
	if options.TopK != nil {
		params.TopK = param.NewOpt(*options.TopK)
	}
	if options.TopP != nil {
		params.TopP = param.NewOpt(*options.TopP)
	}
	if len(options.Stop) > 0 {
		params.StopSequences = slices.Clone(options.Stop)
	}

	system, messages, err := mapProtocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	params.System = append(params.System, system...)
	params.Messages = append(params.Messages, messages...)
	tools, err := mapProtocolTools(req.Tools)
	if err != nil {
		return nil, err
	}
	params.Tools = append(params.Tools, tools...)
	applyProtocolPromptCaching(&params)
	return &params, nil
}

func cloneProtocolOptions(options corechat.Options) corechat.Options {
	clone := options
	clone.Stop = slices.Clone(options.Stop)
	clone.FrequencyPenalty = cloneProtocolPointer(options.FrequencyPenalty)
	clone.MaxTokens = cloneProtocolPointer(options.MaxTokens)
	clone.PresencePenalty = cloneProtocolPointer(options.PresencePenalty)
	clone.Temperature = cloneProtocolPointer(options.Temperature)
	clone.TopK = cloneProtocolPointer(options.TopK)
	clone.TopP = cloneProtocolPointer(options.TopP)
	return clone
}

func cloneProtocolPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mergeProtocolOptions(defaults, overrides corechat.Options) corechat.Options {
	merged := cloneProtocolOptions(defaults)
	if overrides.Model != "" {
		merged.Model = overrides.Model
	}
	if overrides.FrequencyPenalty != nil {
		merged.FrequencyPenalty = overrides.FrequencyPenalty
	}
	if overrides.MaxTokens != nil {
		merged.MaxTokens = overrides.MaxTokens
	}
	if overrides.PresencePenalty != nil {
		merged.PresencePenalty = overrides.PresencePenalty
	}
	if len(overrides.Stop) > 0 {
		merged.Stop = slices.Clone(overrides.Stop)
	}
	if overrides.Temperature != nil {
		merged.Temperature = overrides.Temperature
	}
	if overrides.TopK != nil {
		merged.TopK = overrides.TopK
	}
	if overrides.TopP != nil {
		merged.TopP = overrides.TopP
	}
	return merged
}

func mapProtocolMessages(messages []corechat.Message) ([]anthropicsdk.TextBlockParam, []anthropicsdk.MessageParam, error) {
	system := make([]anthropicsdk.TextBlockParam, 0)
	conversation := make([]anthropicsdk.MessageParam, 0, len(messages))
	for i := range messages {
		message := messages[i]
		switch message.Role {
		case corechat.RoleSystem:
			for j := range message.Parts {
				system = append(system, anthropicsdk.TextBlockParam{Text: message.Parts[j].Text})
			}
		case corechat.RoleUser:
			blocks, err := mapProtocolUserParts(message.Parts)
			if err != nil {
				return nil, nil, fmt.Errorf("anthropic: messages[%d]: %w", i, err)
			}
			conversation = append(conversation, anthropicsdk.NewUserMessage(blocks...))
		case corechat.RoleAssistant:
			blocks, err := mapProtocolAssistant(message)
			if err != nil {
				return nil, nil, fmt.Errorf("anthropic: messages[%d]: %w", i, err)
			}
			conversation = append(conversation, anthropicsdk.NewAssistantMessage(blocks...))
		case corechat.RoleTool:
			blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(message.Parts))
			for j := range message.Parts {
				result := message.Parts[j].ToolResult
				blocks = append(blocks, anthropicsdk.NewToolResultBlock(result.ID, result.Result, result.IsError))
			}
			conversation = append(conversation, anthropicsdk.NewUserMessage(blocks...))
		default:
			return nil, nil, fmt.Errorf("anthropic: messages[%d]: unsupported role %q", i, message.Role)
		}
	}
	return system, conversation, nil
}

func mapProtocolUserParts(parts []corechat.Part) ([]anthropicsdk.ContentBlockParamUnion, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(parts))
	for i := range parts {
		switch parts[i].Kind {
		case corechat.PartText:
			blocks = append(blocks, anthropicsdk.NewTextBlock(parts[i].Text))
		case corechat.PartMedia:
			block, err := mapProtocolMedia(parts[i].Media)
			if err != nil {
				return nil, fmt.Errorf("parts[%d]: %w", i, err)
			}
			blocks = append(blocks, block)
		default:
			return nil, fmt.Errorf("parts[%d]: unsupported user part %q", i, parts[i].Kind)
		}
	}
	return blocks, nil
}

func mapProtocolMedia(value *media.Media) (anthropicsdk.ContentBlockParamUnion, error) {
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil {
		return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("media MIME %q: %w", value.MIME, err)
	}
	if strings.HasPrefix(mediaType, "image/") {
		switch value.Source.Kind {
		case media.SourceBytes:
			if !protocolImageMIME(mediaType) {
				return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("image MIME %q is unsupported", mediaType)
			}
			data, bytesErr := value.Bytes()
			if bytesErr != nil {
				return anthropicsdk.ContentBlockParamUnion{}, bytesErr
			}
			return anthropicsdk.NewImageBlockBase64(mediaType, base64.StdEncoding.EncodeToString(data)), nil
		case media.SourceURI:
			uri, uriErr := value.URI()
			if uriErr != nil {
				return anthropicsdk.ContentBlockParamUnion{}, uriErr
			}
			return anthropicsdk.NewImageBlock(anthropicsdk.URLImageSourceParam{URL: uri}), nil
		default:
			return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("image source %q is unsupported", value.Source.Kind)
		}
	}
	if mediaType == "application/pdf" {
		var block anthropicsdk.ContentBlockParamUnion
		switch value.Source.Kind {
		case media.SourceBytes:
			data, bytesErr := value.Bytes()
			if bytesErr != nil {
				return anthropicsdk.ContentBlockParamUnion{}, bytesErr
			}
			block = anthropicsdk.NewDocumentBlock(anthropicsdk.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(data)})
		case media.SourceURI:
			uri, uriErr := value.URI()
			if uriErr != nil {
				return anthropicsdk.ContentBlockParamUnion{}, uriErr
			}
			block = anthropicsdk.NewDocumentBlock(anthropicsdk.URLPDFSourceParam{URL: uri})
		default:
			return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("PDF source %q is unsupported", value.Source.Kind)
		}
		if value.Name != "" {
			block.OfDocument.Title = param.NewOpt(value.Name)
		}
		return block, nil
	}
	return anthropicsdk.ContentBlockParamUnion{}, fmt.Errorf("media MIME %q is unsupported", mediaType)
}

func protocolImageMIME(mediaType string) bool {
	switch mediaType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func mapProtocolAssistant(message corechat.Message) ([]anthropicsdk.ContentBlockParamUnion, error) {
	redacted, err := protocolRedactedReasoning(message.Metadata)
	if err != nil {
		return nil, err
	}
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(redacted)+len(message.Parts))
	for _, value := range redacted {
		blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(value))
	}
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			blocks = append(blocks, anthropicsdk.NewTextBlock(part.Text))
		case corechat.PartReasoning:
			if len(part.Signature) == 0 {
				return nil, fmt.Errorf("parts[%d]: reasoning signature is required for replay", i)
			}
			blocks = append(blocks, anthropicsdk.NewThinkingBlock(string(part.Signature), part.Text))
		case corechat.PartToolCall:
			var input any
			if part.ToolCall.Arguments == "" {
				input = map[string]any{}
			} else if err := json.Unmarshal([]byte(part.ToolCall.Arguments), &input); err != nil {
				return nil, fmt.Errorf("parts[%d].tool_call.arguments: %w", i, err)
			}
			blocks = append(blocks, anthropicsdk.NewToolUseBlock(part.ToolCall.ID, input, part.ToolCall.Name))
		default:
			return nil, fmt.Errorf("parts[%d]: unsupported assistant part %q", i, part.Kind)
		}
	}
	return blocks, nil
}

func protocolRedactedReasoning(values metadata.Map) ([]string, error) {
	raw, ok := values[protocolRedactedReasoningKey]
	if !ok {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil {
		return nil, fmt.Errorf("metadata %q must be a string or string array", protocolRedactedReasoningKey)
	}
	for i, value := range multiple {
		if value == "" {
			return nil, fmt.Errorf("metadata %q[%d] is empty", protocolRedactedReasoningKey, i)
		}
	}
	return multiple, nil
}

func mapProtocolTools(definitions []corechat.ToolDefinition) ([]anthropicsdk.ToolUnionParam, error) {
	tools := make([]anthropicsdk.ToolUnionParam, 0, len(definitions))
	for i := range definitions {
		var schema map[string]any
		if err := json.Unmarshal(definitions[i].InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("anthropic: tools[%d].input_schema: %w", i, err)
		}
		delete(schema, "type")
		tool := anthropicsdk.ToolParam{
			Name:        definitions[i].Name,
			Description: param.NewOpt(definitions[i].Description),
			InputSchema: anthropicsdk.ToolInputSchemaParam{ExtraFields: schema},
		}
		tools = append(tools, anthropicsdk.ToolUnionParam{OfTool: &tool})
	}
	return tools, nil
}

func applyProtocolPromptCaching(params *anthropicsdk.MessageNewParams) {
	if len(params.Tools) == 0 || protocolHasCacheControl(params) {
		return
	}
	if cacheControl := params.Tools[len(params.Tools)-1].GetCacheControl(); cacheControl != nil {
		*cacheControl = anthropicsdk.NewCacheControlEphemeralParam()
	}
	if len(params.Messages) == 0 {
		return
	}
	content := params.Messages[len(params.Messages)-1].Content
	if len(content) == 0 {
		return
	}
	if cacheControl := content[len(content)-1].GetCacheControl(); cacheControl != nil {
		*cacheControl = anthropicsdk.NewCacheControlEphemeralParam()
	}
}

func protocolHasCacheControl(params *anthropicsdk.MessageNewParams) bool {
	if !param.IsOmitted(params.CacheControl) {
		return true
	}
	for i := range params.System {
		if !param.IsOmitted(params.System[i].CacheControl) {
			return true
		}
	}
	for i := range params.Tools {
		if value := params.Tools[i].GetCacheControl(); value != nil && !param.IsOmitted(*value) {
			return true
		}
	}
	for i := range params.Messages {
		for j := range params.Messages[i].Content {
			if value := params.Messages[i].Content[j].GetCacheControl(); value != nil && !param.IsOmitted(*value) {
				return true
			}
		}
	}
	return false
}
