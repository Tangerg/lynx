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
	// RequestExtensionKey stores provider-owned top-level Messages API fields in
	// a Core request for fields without a provider-neutral equivalent.
	RequestExtensionKey          = "anthropic/request"
	protocolReasoningKindKey     = "anthropic/reasoning_kind"
	protocolReasoningProviderKey = "anthropic/reasoning_provider"
	protocolReasoningThinking    = "thinking"
	protocolReasoningRedacted    = "redacted_thinking"
	protocolNativeStopReasonKey  = "anthropic/native_stop_reason"
	protocolStopSequenceKey      = "anthropic/stop_sequence"
	protocolUsageKey             = "anthropic/usage"
)

func mapProtocolRequest(defaults corechat.Options, req *corechat.Request, dialect Dialect) (*anthropicsdk.MessageNewParams, error) {
	extensionKey := protocolRequestExtensionKey(dialect.Provider)
	fields, _, err := metadata.Decode[map[string]any](req.Options.Extensions, extensionKey)
	if err != nil {
		return nil, fmt.Errorf("anthropic: extension %q: %w", extensionKey, err)
	}
	for _, name := range []string{"model", "messages", "system", "tools", "max_tokens", "temperature", "top_k", "top_p", "stop_sequences"} {
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("anthropic: extension %q field %q is owned by Core", extensionKey, name)
		}
	}
	params := anthropicsdk.MessageNewParams{}
	params.SetExtraFields(fields)

	options, err := defaults.Merged(req.Options)
	if err != nil {
		return nil, fmt.Errorf("anthropic: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("anthropic: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil {
		return nil, errors.New("anthropic: options.frequency_penalty is not supported")
	}
	if options.PresencePenalty != nil {
		return nil, errors.New("anthropic: options.presence_penalty is not supported")
	}
	if dialect.RejectTopK && options.TopK != nil {
		return nil, errors.New("anthropic: options.top_k is not supported by current Claude models")
	}
	if dialect.RejectTopP && options.TopP != nil {
		return nil, errors.New("anthropic: options.top_p is not supported by current Claude models")
	}
	if dialect.MaxTemperature > 0 && options.Temperature != nil && *options.Temperature > dialect.MaxTemperature {
		return nil, fmt.Errorf("anthropic: options.temperature must be between 0 and %g", dialect.MaxTemperature)
	}
	params.Model = options.Model
	if options.MaxTokens != nil {
		params.MaxTokens = *options.MaxTokens
	} else {
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

	system, messages, err := mapProtocolMessages(req.Messages, dialect.Provider)
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
	return &params, nil
}

func mapProtocolMessages(messages []corechat.Message, provider string) ([]anthropicsdk.TextBlockParam, []anthropicsdk.MessageParam, error) {
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
			blocks, err := mapProtocolAssistant(message, provider)
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

func mapProtocolAssistant(message corechat.Message, provider string) ([]anthropicsdk.ContentBlockParamUnion, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(message.Parts))
	for i := range message.Parts {
		part := message.Parts[i]
		switch part.Kind {
		case corechat.PartText:
			blocks = append(blocks, anthropicsdk.NewTextBlock(part.Text))
		case corechat.PartReasoning:
			issuer, issuerFound, err := metadata.Decode[string](part.Metadata, protocolReasoningProviderKey)
			if err != nil {
				return nil, fmt.Errorf("parts[%d].metadata[%q]: %w", i, protocolReasoningProviderKey, err)
			}
			if !issuerFound || issuer != provider {
				// Opaque state is valid only for the provider that issued it.
				continue
			}
			kind, found, err := metadata.Decode[string](part.Metadata, protocolReasoningKindKey)
			if err != nil {
				return nil, fmt.Errorf("parts[%d].metadata[%q]: %w", i, protocolReasoningKindKey, err)
			}
			if !found {
				// Unsigned or foreign-provider reasoning is visible Core state,
				// not valid Anthropic replay state.
				continue
			}
			switch kind {
			case protocolReasoningThinking:
				if len(part.Signature) == 0 {
					return nil, fmt.Errorf("parts[%d]: Anthropic thinking requires its provider signature", i)
				}
				blocks = append(blocks, anthropicsdk.NewThinkingBlock(string(part.Signature), part.Text))
			case protocolReasoningRedacted:
				if len(part.Signature) == 0 {
					return nil, fmt.Errorf("parts[%d]: Anthropic redacted thinking requires opaque data", i)
				}
				blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(string(part.Signature)))
			default:
				return nil, fmt.Errorf("parts[%d].metadata[%q]: unknown kind %q", i, protocolReasoningKindKey, kind)
			}
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
