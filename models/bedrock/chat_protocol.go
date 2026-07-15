package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	// ChatRequestExtensionKey stores [ChatRequestOptions] in a Core request.
	ChatRequestExtensionKey   = "bedrock/request"
	chatNativeFinishReasonKey = "bedrock/native_finish_reason"
)

// ChatRequestOptions carries serializable Bedrock Converse fields that have no
// provider-neutral Core equivalent. Common model, message, tool, and sampling
// fields are always derived from the Core request and take precedence.
type ChatRequestOptions struct {
	AdditionalModelRequestFields      map[string]any                      `json:"additional_model_request_fields,omitempty"`
	AdditionalModelResponseFieldPaths []string                            `json:"additional_model_response_field_paths,omitempty"`
	GuardrailConfig                   *types.GuardrailConfiguration       `json:"guardrail_config,omitempty"`
	StreamGuardrailConfig             *types.GuardrailStreamConfiguration `json:"stream_guardrail_config,omitempty"`
	OutputConfig                      *types.OutputConfig                 `json:"output_config,omitempty"`
	PerformanceConfig                 *types.PerformanceConfiguration     `json:"performance_config,omitempty"`
	RequestMetadata                   map[string]string                   `json:"request_metadata,omitempty"`
	ServiceTier                       *types.ServiceTier                  `json:"service_tier,omitempty"`
}

// ChatConfig configures the Bedrock Converse Core chat adapter.
type ChatConfig struct {
	DefaultOptions corechat.Options
	Region         string
	AWSConfig      *aws.Config
}

// Validate verifies construction-time configuration without loading AWS
// credentials or performing network I/O.
func (c ChatConfig) Validate() error {
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("bedrock: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Core chat through Bedrock's provider-neutral Converse API.
type Chat struct {
	api      *API
	defaults corechat.Options
}

// NewChat constructs a Bedrock Converse Core chat adapter.
func NewChat(ctx context.Context, cfg ChatConfig) (*Chat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(ctx, APIConfig{Region: cfg.Region, AWSConfig: cfg.AWSConfig})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cloneChatOptions(cfg.DefaultOptions)}, nil
}

// Call performs one Bedrock Converse request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	input, model, err := c.buildConverseInput(req)
	if err != nil {
		return nil, err
	}
	output, err := c.api.Converse(ctx, input)
	if err != nil {
		return nil, err
	}
	return mapProtocolConverseResponse(model, output)
}

// Stream performs one Bedrock ConverseStream request and yields validated
// provider deltas with cumulative usage snapshots.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		input, model, err := c.buildConverseStreamInput(req)
		if err != nil {
			yield(nil, err)
			return
		}
		output, err := c.api.ConverseStream(ctx, input)
		if err != nil {
			yield(nil, err)
			return
		}
		stream := output.GetStream()
		defer stream.Close()

		state := newProtocolChunkAccumulator(model)
		for event := range stream.Events() {
			response, include, mapErr := state.add(event)
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if include && !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, streamErr)
		}
	}
}

func (c *Chat) buildConverseInput(req *corechat.Request) (*bedrockruntime.ConverseInput, string, error) {
	prepared, err := c.prepareRequest(req)
	if err != nil {
		return nil, "", err
	}
	return &bedrockruntime.ConverseInput{
		ModelId:                           aws.String(prepared.model),
		AdditionalModelRequestFields:      toDocument(prepared.native.AdditionalModelRequestFields),
		AdditionalModelResponseFieldPaths: slices.Clone(prepared.native.AdditionalModelResponseFieldPaths),
		GuardrailConfig:                   prepared.native.GuardrailConfig,
		InferenceConfig:                   prepared.inference,
		Messages:                          prepared.messages,
		OutputConfig:                      prepared.native.OutputConfig,
		PerformanceConfig:                 prepared.native.PerformanceConfig,
		RequestMetadata:                   maps.Clone(prepared.native.RequestMetadata),
		ServiceTier:                       prepared.native.ServiceTier,
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
		AdditionalModelRequestFields:      toDocument(prepared.native.AdditionalModelRequestFields),
		AdditionalModelResponseFieldPaths: slices.Clone(prepared.native.AdditionalModelResponseFieldPaths),
		GuardrailConfig:                   prepared.native.StreamGuardrailConfig,
		InferenceConfig:                   prepared.inference,
		Messages:                          prepared.messages,
		OutputConfig:                      prepared.native.OutputConfig,
		PerformanceConfig:                 prepared.native.PerformanceConfig,
		RequestMetadata:                   maps.Clone(prepared.native.RequestMetadata),
		ServiceTier:                       prepared.native.ServiceTier,
		System:                            prepared.system,
		ToolConfig:                        prepared.tools,
	}, prepared.model, nil
}

type preparedChatRequest struct {
	model     string
	system    []types.SystemContentBlock
	messages  []types.Message
	inference *types.InferenceConfiguration
	tools     *types.ToolConfiguration
	native    ChatRequestOptions
}

func (c *Chat) prepareRequest(req *corechat.Request) (*preparedChatRequest, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("bedrock: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("bedrock: request: %w", err)
	}
	options := mergeChatOptions(c.defaults, req.Options)
	if options.Model == "" {
		return nil, errors.New("bedrock: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil || options.PresencePenalty != nil || options.TopK != nil {
		return nil, errors.New("bedrock: frequency_penalty, presence_penalty, and top_k are not supported by Converse inference configuration")
	}

	native, found, err := metadata.Decode[ChatRequestOptions](req.Extensions, ChatRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("bedrock: extension %q: %w", ChatRequestExtensionKey, err)
	}
	if !found {
		native = ChatRequestOptions{}
	}

	system, messages, err := mapProtocolMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	tools, err := mapProtocolTools(req.Tools)
	if err != nil {
		return nil, err
	}
	return &preparedChatRequest{
		model:     options.Model,
		system:    system,
		messages:  messages,
		inference: mapInferenceOptions(options),
		tools:     tools,
		native:    native,
	}, nil
}

func cloneChatOptions(options corechat.Options) corechat.Options {
	clone := options
	clone.Stop = slices.Clone(options.Stop)
	clone.FrequencyPenalty = clonePointer(options.FrequencyPenalty)
	clone.MaxTokens = clonePointer(options.MaxTokens)
	clone.PresencePenalty = clonePointer(options.PresencePenalty)
	clone.Temperature = clonePointer(options.Temperature)
	clone.TopK = clonePointer(options.TopK)
	clone.TopP = clonePointer(options.TopP)
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func mergeChatOptions(defaults, overrides corechat.Options) corechat.Options {
	merged := cloneChatOptions(defaults)
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
	if len(overrides.Stop) != 0 {
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
			block, err := mapProtocolPart(message.Parts[partIndex])
			if err != nil {
				return nil, nil, fmt.Errorf("bedrock: messages[%d].parts[%d]: %w", messageIndex, partIndex, err)
			}
			blocks = append(blocks, block)
		}
		result = append(result, types.Message{Role: role, Content: blocks})
	}
	return system, result, nil
}

func mapProtocolPart(part corechat.Part) (types.ContentBlock, error) {
	switch part.Kind {
	case corechat.PartText:
		return &types.ContentBlockMemberText{Value: part.Text}, nil
	case corechat.PartMedia:
		block := mediaToBlock(part.Media)
		if block == nil {
			return nil, fmt.Errorf("unsupported media MIME type %q", part.Media.MIME)
		}
		return block, nil
	case corechat.PartReasoning:
		reasoning := types.ReasoningTextBlock{}
		if part.Text != "" {
			reasoning.Text = aws.String(part.Text)
		}
		if len(part.Signature) != 0 {
			reasoning.Signature = aws.String(string(part.Signature))
		}
		return &types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberReasoningText{Value: reasoning}}, nil
	case corechat.PartToolCall:
		var arguments any
		if part.ToolCall.Arguments != "" {
			if err := json.Unmarshal([]byte(part.ToolCall.Arguments), &arguments); err != nil {
				return nil, fmt.Errorf("tool call arguments: %w", err)
			}
		}
		return &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String(part.ToolCall.ID),
			Name:      aws.String(part.ToolCall.Name),
			Input:     toDocument(arguments),
		}}, nil
	case corechat.PartToolResult:
		status := types.ToolResultStatusSuccess
		if part.ToolResult.IsError {
			status = types.ToolResultStatusError
		}
		return &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
			ToolUseId: aws.String(part.ToolResult.ID),
			Status:    status,
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{Value: part.ToolResult.Result},
			},
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported part kind %q", part.Kind)
	}
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
			InputSchema: &types.ToolInputSchemaMemberJson{Value: toDocument(schema)},
		}})
	}
	return &types.ToolConfiguration{Tools: tools}, nil
}

func mapProtocolConverseResponse(model string, output *bedrockruntime.ConverseOutput) (*corechat.Response, error) {
	if output == nil || output.Output == nil {
		return nil, errors.New("bedrock: response has no output")
	}
	messageOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok || messageOutput == nil {
		return nil, errors.New("bedrock: response has no message output")
	}
	parts, err := mapProtocolResponseBlocks(messageOutput.Value.Content)
	if err != nil {
		return nil, err
	}
	choice := corechat.Choice{Index: 0, FinishReason: mapProtocolStopReason(output.StopReason)}
	if len(parts) != 0 {
		message := corechat.NewAssistantMessage(parts...)
		choice.Message = &message
	}
	if choice.FinishReason == corechat.FinishReasonOther {
		if err := choice.SetExtension(chatNativeFinishReasonKey, string(output.StopReason)); err != nil {
			return nil, err
		}
	}
	response := &corechat.Response{Model: model, Choices: []corechat.Choice{choice}, Usage: mapProtocolUsage(output.Usage)}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("bedrock: response: %w", err)
	}
	return response, nil
}

func mapProtocolResponseBlocks(blocks []types.ContentBlock) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(blocks))
	for index := range blocks {
		switch block := blocks[index].(type) {
		case *types.ContentBlockMemberText:
			if block.Value != "" {
				parts = append(parts, corechat.NewTextPart(block.Value))
			}
		case *types.ContentBlockMemberReasoningContent:
			reasoning, ok := block.Value.(*types.ReasoningContentBlockMemberReasoningText)
			if !ok {
				continue
			}
			var text string
			var signature []byte
			if reasoning.Value.Text != nil {
				text = *reasoning.Value.Text
			}
			if reasoning.Value.Signature != nil {
				signature = []byte(*reasoning.Value.Signature)
			}
			if text != "" || len(signature) != 0 {
				parts = append(parts, corechat.NewReasoningPart(text, signature))
			}
		case *types.ContentBlockMemberToolUse:
			if block.Value.ToolUseId == nil || block.Value.Name == nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: tool use lacks ID or name", index)
			}
			arguments, err := json.Marshal(block.Value.Input)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: tool arguments: %w", index, err)
			}
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
				ID: *block.Value.ToolUseId, Name: *block.Value.Name, Arguments: string(arguments),
			}))
		}
	}
	return parts, nil
}

func mapProtocolStopReason(reason types.StopReason) corechat.FinishReason {
	switch reason {
	case types.StopReasonEndTurn, types.StopReasonStopSequence:
		return corechat.FinishReasonStop
	case types.StopReasonMaxTokens:
		return corechat.FinishReasonLength
	case types.StopReasonToolUse:
		return corechat.FinishReasonToolCalls
	case types.StopReasonContentFiltered, types.StopReasonGuardrailIntervened:
		return corechat.FinishReasonContentFilter
	default:
		return corechat.FinishReasonOther
	}
}

func mapProtocolUsage(usage *types.TokenUsage) corechat.Usage {
	if usage == nil {
		return corechat.Usage{}
	}
	result := corechat.Usage{}
	if usage.InputTokens != nil {
		result.InputTokens = int64(*usage.InputTokens)
	}
	if usage.OutputTokens != nil {
		result.OutputTokens = int64(*usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != nil {
		value := int64(*usage.CacheReadInputTokens)
		result.CacheReadInputTokens = &value
	}
	if usage.CacheWriteInputTokens != nil {
		value := int64(*usage.CacheWriteInputTokens)
		result.CacheWriteInputTokens = &value
	}
	return result
}

type protocolToolIdentity struct {
	id   string
	name string
}

type protocolChunkAccumulator struct {
	model string
	tools map[int32]protocolToolIdentity
}

func newProtocolChunkAccumulator(model string) *protocolChunkAccumulator {
	return &protocolChunkAccumulator{model: model, tools: make(map[int32]protocolToolIdentity)}
}

func (a *protocolChunkAccumulator) add(event types.ConverseStreamOutput) (*corechat.Response, bool, error) {
	response := &corechat.Response{Model: a.model}
	var choice *corechat.Choice

	switch typed := event.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		tool, ok := typed.Value.Start.(*types.ContentBlockStartMemberToolUse)
		if !ok || typed.Value.ContentBlockIndex == nil || tool.Value.ToolUseId == nil || tool.Value.Name == nil {
			return nil, false, nil
		}
		identity := protocolToolIdentity{id: *tool.Value.ToolUseId, name: *tool.Value.Name}
		a.tools[*typed.Value.ContentBlockIndex] = identity
		part := corechat.NewToolCallPart(corechat.ToolCall{ID: identity.id, Name: identity.name})
		message := corechat.NewAssistantMessage(part)
		choice = &corechat.Choice{Index: 0, Message: &message}
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		part, include, err := a.mapDelta(typed.Value)
		if err != nil || !include {
			return nil, false, err
		}
		message := corechat.NewAssistantMessage(part)
		choice = &corechat.Choice{Index: 0, Message: &message}
	case *types.ConverseStreamOutputMemberMessageStop:
		choice = &corechat.Choice{Index: 0, FinishReason: mapProtocolStopReason(typed.Value.StopReason)}
		if choice.FinishReason == corechat.FinishReasonOther {
			if err := choice.SetExtension(chatNativeFinishReasonKey, string(typed.Value.StopReason)); err != nil {
				return nil, false, err
			}
		}
	case *types.ConverseStreamOutputMemberMetadata:
		if typed.Value.Usage == nil {
			return nil, false, nil
		}
		response.Usage = mapProtocolUsage(typed.Value.Usage)
	default:
		return nil, false, nil
	}

	if choice != nil {
		response.Choices = []corechat.Choice{*choice}
	}
	if err := response.Validate(); err != nil {
		return nil, false, fmt.Errorf("bedrock: stream response: %w", err)
	}
	return response, true, nil
}

func (a *protocolChunkAccumulator) mapDelta(delta types.ContentBlockDeltaEvent) (corechat.Part, bool, error) {
	switch value := delta.Delta.(type) {
	case *types.ContentBlockDeltaMemberText:
		if value.Value == "" {
			return corechat.Part{}, false, nil
		}
		return corechat.NewTextPart(value.Value), true, nil
	case *types.ContentBlockDeltaMemberReasoningContent:
		switch reasoning := value.Value.(type) {
		case *types.ReasoningContentBlockDeltaMemberText:
			if reasoning.Value == "" {
				return corechat.Part{}, false, nil
			}
			return corechat.NewReasoningPart(reasoning.Value, nil), true, nil
		case *types.ReasoningContentBlockDeltaMemberSignature:
			if reasoning.Value == "" {
				return corechat.Part{}, false, nil
			}
			return corechat.NewReasoningPart("", []byte(reasoning.Value)), true, nil
		}
	case *types.ContentBlockDeltaMemberToolUse:
		if value.Value.Input == nil || *value.Value.Input == "" || delta.ContentBlockIndex == nil {
			return corechat.Part{}, false, nil
		}
		identity, ok := a.tools[*delta.ContentBlockIndex]
		if !ok {
			return corechat.Part{}, false, fmt.Errorf("bedrock: tool delta for unknown content block %d", *delta.ContentBlockIndex)
		}
		return corechat.NewToolCallPart(corechat.ToolCall{
			ID: identity.id, Name: identity.name, Arguments: *value.Value.Input,
		}), true, nil
	}
	return corechat.Part{}, false, nil
}
