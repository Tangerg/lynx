package bedrock

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/scope/core/chat"
)

func (c *Chat) buildConverseInput(req *corechat.Request) (*bedrockruntime.ConverseInput, string, error) {
	prepared, err := c.prepareRequest(req)
	if err != nil {
		return nil, "", err
	}
	return &bedrockruntime.ConverseInput{
		ModelId:                           aws.String(prepared.model),
		AdditionalModelRequestFields:      toBedrockDocument(prepared.native.AdditionalModelRequestFields),
		AdditionalModelResponseFieldPaths: slices.Clone(prepared.native.AdditionalModelResponseFieldPaths),
		GuardrailConfig:                   mapGuardrailOptions(prepared.native.Guardrail),
		InferenceConfig:                   prepared.inference,
		Messages:                          prepared.messages,
		OutputConfig:                      mapOutputFormat(prepared.outputFormat),
		PerformanceConfig:                 mapPerformanceOptions(prepared.native.PerformanceLatency),
		RequestMetadata:                   maps.Clone(prepared.native.RequestMetadata),
		ServiceTier:                       mapServiceTier(prepared.native.ServiceTier),
		System:                            prepared.system,
		ToolConfig:                        prepared.tools,
	}, prepared.model, nil
}

func (c *Chat) buildConverseStreamInput(req *corechat.Request) (*bedrockruntime.ConverseStreamInput, string, error) {
	prepared, err := c.prepareRequest(req)
	if err != nil {
		return nil, "", err
	}
	return &bedrockruntime.ConverseStreamInput{
		ModelId:                           aws.String(prepared.model),
		AdditionalModelRequestFields:      toBedrockDocument(prepared.native.AdditionalModelRequestFields),
		AdditionalModelResponseFieldPaths: slices.Clone(prepared.native.AdditionalModelResponseFieldPaths),
		GuardrailConfig:                   mapStreamGuardrailOptions(prepared.native.StreamGuardrail),
		InferenceConfig:                   prepared.inference,
		Messages:                          prepared.messages,
		OutputConfig:                      mapOutputFormat(prepared.outputFormat),
		PerformanceConfig:                 mapPerformanceOptions(prepared.native.PerformanceLatency),
		RequestMetadata:                   maps.Clone(prepared.native.RequestMetadata),
		ServiceTier:                       mapServiceTier(prepared.native.ServiceTier),
		System:                            prepared.system,
		ToolConfig:                        prepared.tools,
	}, prepared.model, nil
}

type preparedChatRequest struct {
	model        string
	system       []types.SystemContentBlock
	messages     []types.Message
	inference    *types.InferenceConfiguration
	tools        *types.ToolConfiguration
	outputFormat *corechat.OutputFormat
	native       ChatRequestOptions
}

func mapGuardrailOptions(options *GuardrailOptions) *types.GuardrailConfiguration {
	if options == nil {
		return nil
	}
	return &types.GuardrailConfiguration{
		GuardrailIdentifier: aws.String(options.Identifier),
		GuardrailVersion:    aws.String(options.Version),
		Trace:               types.GuardrailTrace(options.Trace),
	}
}

func mapStreamGuardrailOptions(options *StreamGuardrailOptions) *types.GuardrailStreamConfiguration {
	if options == nil {
		return nil
	}
	return &types.GuardrailStreamConfiguration{
		GuardrailIdentifier:  aws.String(options.Identifier),
		GuardrailVersion:     aws.String(options.Version),
		Trace:                types.GuardrailTrace(options.Trace),
		StreamProcessingMode: types.GuardrailStreamProcessingMode(options.ProcessingMode),
	}
}

func mapOutputFormat(format *corechat.OutputFormat) *types.OutputConfig {
	if format == nil || format.Type != corechat.OutputFormatJSONSchema {
		return nil
	}
	schema := string(format.Schema)
	return &types.OutputConfig{TextFormat: &types.OutputFormat{
		Type: types.OutputFormatTypeJsonSchema,
		Structure: &types.OutputFormatStructureMemberJsonSchema{Value: types.JsonSchemaDefinition{
			Name:        aws.String(format.Name),
			Description: aws.String(format.Description),
			Schema:      aws.String(schema),
		}},
	}}
}

func mapPerformanceOptions(latency string) *types.PerformanceConfiguration {
	if latency == "" {
		return nil
	}
	return &types.PerformanceConfiguration{Latency: types.PerformanceConfigLatency(latency)}
}

func mapServiceTier(tier string) *types.ServiceTier {
	if tier == "" {
		return nil
	}
	return &types.ServiceTier{Type: types.ServiceTierType(tier)}
}

func (c *Chat) prepareRequest(req *corechat.Request) (*preparedChatRequest, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("bedrock: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("bedrock: request: %w", err)
	}
	options, err := c.defaults.Resolve(req.Options)
	if err != nil {
		return nil, fmt.Errorf("bedrock: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("bedrock: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil || options.PresencePenalty != nil || options.TopK != nil {
		return nil, errors.New("bedrock: frequency_penalty, presence_penalty, and top_k are not supported by Converse inference configuration")
	}

	native, found, err := req.Options.Extensions.Decode[ChatRequestOptions](ChatRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("bedrock: extension %q: %w", ChatRequestExtensionKey, err)
	}
	if !found {
		native = ChatRequestOptions{}
	} else {
		fields, _, decodeErr := req.Options.Extensions.Decode[map[string]json.RawMessage](ChatRequestExtensionKey)
		if decodeErr != nil {
			return nil, fmt.Errorf("bedrock: extension %q: %w", ChatRequestExtensionKey, decodeErr)
		}
		if _, exists := fields["json_schema"]; exists {
			return nil, fmt.Errorf("bedrock: extension %q field %q is owned by options.output_format", ChatRequestExtensionKey, "json_schema")
		}
	}

	system, messages, err := mapProtocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	tools, err := mapProtocolTools(req.Tools)
	if err != nil {
		return nil, err
	}
	if options.OutputFormat != nil && options.OutputFormat.Type == corechat.OutputFormatJSON {
		instruction, err := options.OutputFormat.FallbackInstruction()
		if err != nil {
			return nil, fmt.Errorf("bedrock: output format fallback: %w", err)
		}
		system = append(system, &types.SystemContentBlockMemberText{Value: instruction})
	}
	return &preparedChatRequest{
		model:        options.Model,
		system:       system,
		messages:     messages,
		inference:    mapInferenceOptions(options),
		tools:        tools,
		outputFormat: options.OutputFormat,
		native:       native,
	}, nil
}

func mapInferenceOptions(options corechat.Options) *types.InferenceConfiguration {
	configuration := &types.InferenceConfiguration{StopSequences: slices.Clone(options.Stop)}
	if options.MaxTokens != nil {
		value := int32(*options.MaxTokens)
		configuration.MaxTokens = &value
	}
	if options.Temperature != nil {
		value := float32(*options.Temperature)
		configuration.Temperature = &value
	}
	if options.TopP != nil {
		value := float32(*options.TopP)
		configuration.TopP = &value
	}
	return configuration
}

func mapProtocolMessages(messages []corechat.Message) ([]types.SystemContentBlock, []types.Message, error) {
	var system []types.SystemContentBlock
	result := make([]types.Message, 0, len(messages))
	for messageIndex := range messages {
		message := messages[messageIndex]
		if message.Role == corechat.RoleSystem {
			for partIndex := range message.Parts {
				system = append(system, &types.SystemContentBlockMemberText{Value: message.Parts[partIndex].Text})
			}
			continue
		}

		role := types.ConversationRoleUser
		if message.Role == corechat.RoleAssistant {
			role = types.ConversationRoleAssistant
		}
		blocks := make([]types.ContentBlock, 0, len(message.Parts))
		for partIndex := range message.Parts {
			block, include, err := mapProtocolPart(message.Parts[partIndex])
			if err != nil {
				return nil, nil, fmt.Errorf("bedrock: messages[%d].parts[%d]: %w", messageIndex, partIndex, err)
			}
			if include {
				blocks = append(blocks, block)
			}
		}
		if len(blocks) == 0 {
			return nil, nil, fmt.Errorf("bedrock: messages[%d]: no parts are valid for Bedrock", messageIndex)
		}
		result = append(result, types.Message{Role: role, Content: blocks})
	}
	return system, result, nil
}

func mapProtocolPart(part corechat.Part) (types.ContentBlock, bool, error) {
	switch part.Kind {
	case corechat.PartText:
		return &types.ContentBlockMemberText{Value: part.Text}, true, nil
	case corechat.PartMedia:
		block, err := mediaToBlock(part.Media)
		if err != nil {
			return nil, false, err
		}
		return block, true, nil
	case corechat.PartReasoning:
		kind, found, err := part.Metadata.Decode[string](chatReasoningKindKey)
		if err != nil {
			return nil, false, err
		}
		if !found {
			// Reasoning from another provider is portable display state, not
			// valid Bedrock replay state.
			return nil, false, nil
		}
		switch kind {
		case chatReasoningText:
			if part.Text == "" || len(part.Signature) == 0 {
				return nil, false, errors.New("bedrock reasoning text requires text and its unmodified signature")
			}
			reasoning := types.ReasoningTextBlock{Text: aws.String(part.Text), Signature: aws.String(string(part.Signature))}
			return &types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberReasoningText{Value: reasoning}}, true, nil
		case chatReasoningRedacted:
			if len(part.Signature) == 0 {
				return nil, false, errors.New("bedrock redacted reasoning requires opaque content")
			}
			return &types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberRedactedContent{Value: slices.Clone(part.Signature)}}, true, nil
		default:
			return nil, false, fmt.Errorf("unknown Bedrock reasoning kind %q", kind)
		}
	case corechat.PartToolCall:
		var arguments any
		if part.ToolCall.Arguments != "" {
			if err := json.Unmarshal([]byte(part.ToolCall.Arguments), &arguments); err != nil {
				return nil, false, fmt.Errorf("tool call arguments: %w", err)
			}
		}
		return &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String(part.ToolCall.ID),
			Name:      aws.String(part.ToolCall.Name),
			Input:     toBedrockDocument(arguments),
		}}, true, nil
	case corechat.PartToolResult:
		status := types.ToolResultStatusSuccess
		if part.ToolResult.IsError {
			status = types.ToolResultStatusError
		}
		content, err := mapToolResultContent(part.ToolResult.Output)
		if err != nil {
			return nil, false, fmt.Errorf("tool result output: %w", err)
		}
		return &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
			ToolUseId: aws.String(part.ToolResult.ID),
			Status:    status,
			Content:   content,
		}}, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported part kind %q", part.Kind)
	}
}

func mapToolResultContent(output corechat.ToolOutput) ([]types.ToolResultContentBlock, error) {
	if len(output.Content) == 0 {
		if len(output.Details) == 0 {
			return []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{}}, nil
		}
		var value any
		if err := json.Unmarshal(output.Details, &value); err != nil {
			return nil, err
		}
		return []types.ToolResultContentBlock{
			&types.ToolResultContentBlockMemberJson{Value: toBedrockDocument(value)},
		}, nil
	}
	content := make([]types.ToolResultContentBlock, 0, len(output.Content))
	for index := range output.Content {
		part := output.Content[index]
		switch part.Kind {
		case corechat.PartText:
			content = append(content, &types.ToolResultContentBlockMemberText{Value: part.Text})
		case corechat.PartMedia:
			mapped, err := mediaToBlock(part.Media)
			if err != nil {
				return nil, fmt.Errorf("content[%d]: %w", index, err)
			}
			switch block := mapped.(type) {
			case *types.ContentBlockMemberImage:
				content = append(content, &types.ToolResultContentBlockMemberImage{Value: block.Value})
			case *types.ContentBlockMemberDocument:
				content = append(content, &types.ToolResultContentBlockMemberDocument{Value: block.Value})
			case *types.ContentBlockMemberVideo:
				content = append(content, &types.ToolResultContentBlockMemberVideo{Value: block.Value})
			default:
				return nil, fmt.Errorf("content[%d]: Bedrock Tool results do not support media type %q", index, part.Media.MIME)
			}
		default:
			return nil, fmt.Errorf("content[%d]: unsupported part %q", index, part.Kind)
		}
	}
	return content, nil
}

func mapProtocolTools(definitions []corechat.ToolDefinition) (*types.ToolConfiguration, error) {
	if len(definitions) == 0 {
		return nil, nil
	}
	tools := make([]types.Tool, 0, len(definitions))
	for index := range definitions {
		var schema any
		if err := json.Unmarshal(definitions[index].InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("bedrock: tools[%d].input_schema: %w", index, err)
		}
		tools = append(tools, &types.ToolMemberToolSpec{Value: types.ToolSpecification{
			Name:        aws.String(definitions[index].Name),
			Description: aws.String(definitions[index].Description),
			InputSchema: &types.ToolInputSchemaMemberJson{Value: toBedrockDocument(schema)},
		}})
	}
	return &types.ToolConfiguration{Tools: tools}, nil
}
