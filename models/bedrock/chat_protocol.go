package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"net/http"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	// ChatRequestExtensionKey stores [ChatRequestOptions] in a Core request.
	ChatRequestExtensionKey = "bedrock/request"
	// ChatResponseExtensionKey preserves the complete official Converse output.
	ChatResponseExtensionKey  = "bedrock/response"
	chatReasoningKindKey      = "bedrock/reasoning_kind"
	chatReasoningText         = "reasoning_text"
	chatReasoningRedacted     = "redacted_content"
	chatNativeFinishReasonKey = "bedrock/native_finish_reason"
)

// ChatRequestOptions carries serializable Bedrock Converse fields that have no
// provider-neutral Core equivalent. Common model, message, tool, and sampling
// fields are always derived from the Core request and take precedence.
type ChatRequestOptions struct {
	AdditionalModelRequestFields      map[string]any          `json:"additional_model_request_fields,omitempty"`
	AdditionalModelResponseFieldPaths []string                `json:"additional_model_response_field_paths,omitempty"`
	Guardrail                         *GuardrailOptions       `json:"guardrail,omitempty"`
	StreamGuardrail                   *StreamGuardrailOptions `json:"stream_guardrail,omitempty"`
	PerformanceLatency                string                  `json:"performance_latency,omitempty"`
	RequestMetadata                   map[string]string       `json:"request_metadata,omitempty"`
	ServiceTier                       string                  `json:"service_tier,omitempty"`
}

// GuardrailOptions configures a Bedrock guardrail without exposing AWS SDK
// wire types.
type GuardrailOptions struct {
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
	Trace      string `json:"trace,omitempty"`
}

// StreamGuardrailOptions adds the streaming processing mode to a guardrail.
type StreamGuardrailOptions struct {
	Identifier     string `json:"identifier"`
	Version        string `json:"version"`
	Trace          string `json:"trace,omitempty"`
	ProcessingMode string `json:"processing_mode,omitempty"`
}

// ChatConfig configures the Bedrock Converse Core chat adapter.
type ChatConfig struct {
	DefaultOptions corechat.Options
	Region         string
	BaseURL        string
	HTTPClient     *http.Client
	Credentials    *Credentials
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
	api      *api
	defaults corechat.Options
}

// NewChat constructs a Bedrock Converse Core chat adapter.
func NewChat(ctx context.Context, cfg ChatConfig) (*Chat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(ctx, apiConfig{
		Region:      cfg.Region,
		BaseURL:     cfg.BaseURL,
		HTTPClient:  cfg.HTTPClient,
		Credentials: cfg.Credentials,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cfg.DefaultOptions.Clone()}, nil
}

// Call performs one Bedrock Converse request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	input, model, err := c.buildConverseInput(req)
	if err != nil {
		return nil, err
	}
	output, err := c.api.converse(ctx, input)
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
		output, err := c.api.converseStream(ctx, input)
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
		AdditionalModelRequestFields:      toDocument(prepared.native.AdditionalModelRequestFields),
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
	options, err := c.defaults.Merged(req.Options)
	if err != nil {
		return nil, fmt.Errorf("bedrock: options: %w", err)
	}
	if options.Model == "" {
		return nil, errors.New("bedrock: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil || options.PresencePenalty != nil || options.TopK != nil {
		return nil, errors.New("bedrock: frequency_penalty, presence_penalty, and top_k are not supported by Converse inference configuration")
	}

	native, found, err := metadata.Decode[ChatRequestOptions](req.Options.Extensions, ChatRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("bedrock: extension %q: %w", ChatRequestExtensionKey, err)
	}
	if !found {
		native = ChatRequestOptions{}
	} else {
		fields, _, decodeErr := metadata.Decode[map[string]json.RawMessage](req.Options.Extensions, ChatRequestExtensionKey)
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
		kind, found, err := metadata.Decode[string](part.Metadata, chatReasoningKindKey)
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
			Input:     toDocument(arguments),
		}}, true, nil
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
		}}, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported part kind %q", part.Kind)
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
	result := &corechat.Result{FinishReason: mapProtocolStopReason(output.StopReason)}
	if len(parts) != 0 {
		message := corechat.NewAssistantMessage(parts...)
		result.Message = &message
	}
	if result.FinishReason == corechat.FinishReasonOther {
		result.Metadata = &corechat.ResultMetadata{}
		if err := result.Metadata.Set(chatNativeFinishReasonKey, string(output.StopReason)); err != nil {
			return nil, err
		}
	}
	response := &corechat.Response{
		Result:   result,
		Metadata: &corechat.ResponseMetadata{Model: model, Usage: mapProtocolUsage(output.Usage)},
	}
	if err := response.Metadata.Set(ChatResponseExtensionKey, output); err != nil {
		return nil, fmt.Errorf("bedrock: preserve native response: %w", err)
	}
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
		case *types.ContentBlockMemberImage:
			value, err := bedrockImageToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberAudio:
			value, err := bedrockAudioToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberVideo:
			value, err := bedrockVideoToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberReasoningContent:
			switch reasoning := block.Value.(type) {
			case *types.ReasoningContentBlockMemberReasoningText:
				if reasoning.Value.Text == nil || reasoning.Value.Signature == nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: reasoning text lacks text or signature", index)
				}
				part, err := NewReasoningPart(*reasoning.Value.Text, []byte(*reasoning.Value.Signature))
				if err != nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
				}
				parts = append(parts, part)
			case *types.ReasoningContentBlockMemberRedactedContent:
				part, err := NewRedactedReasoningPart(reasoning.Value)
				if err != nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
				}
				parts = append(parts, part)
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
	response := &corechat.Response{Metadata: &corechat.ResponseMetadata{Model: a.model}}
	var result *corechat.Result

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
		result = &corechat.Result{Message: &message}
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		part, include, err := a.mapDelta(typed.Value)
		if err != nil || !include {
			return nil, false, err
		}
		message := corechat.NewAssistantMessage(part)
		result = &corechat.Result{Message: &message}
	case *types.ConverseStreamOutputMemberMessageStop:
		result = &corechat.Result{FinishReason: mapProtocolStopReason(typed.Value.StopReason)}
		if result.FinishReason == corechat.FinishReasonOther {
			result.Metadata = &corechat.ResultMetadata{}
			if err := result.Metadata.Set(chatNativeFinishReasonKey, string(typed.Value.StopReason)); err != nil {
				return nil, false, err
			}
		}
	case *types.ConverseStreamOutputMemberMetadata:
		if typed.Value.Usage == nil {
			return nil, false, nil
		}
		response.Metadata.Usage = mapProtocolUsage(typed.Value.Usage)
	default:
		return nil, false, nil
	}

	if result != nil {
		response.Result = result
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
			part := corechat.NewReasoningPart(reasoning.Value, nil)
			if err := setReasoningKind(&part, chatReasoningText); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
		case *types.ReasoningContentBlockDeltaMemberSignature:
			if reasoning.Value == "" {
				return corechat.Part{}, false, nil
			}
			part := corechat.NewReasoningPart("", []byte(reasoning.Value))
			if err := setReasoningKind(&part, chatReasoningText); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
		case *types.ReasoningContentBlockDeltaMemberRedactedContent:
			if len(reasoning.Value) == 0 {
				return corechat.Part{}, false, nil
			}
			part := corechat.NewReasoningPart("", reasoning.Value)
			if err := setReasoningKind(&part, chatReasoningRedacted); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
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
