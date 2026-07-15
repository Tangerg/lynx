package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"mime"
	"slices"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	requestExtensionKey     = "openai/request"
	nativeFinishReasonKey   = "openai/native_finish_reason"
	choiceLogprobsKey       = "openai/logprobs"
	choiceRefusalDeltaKey   = "openai/refusal_delta"
	messageRefusalKey       = "openai/refusal"
	messageAnnotationsKey   = "openai/annotations"
	messageAudioIDKey       = "openai/audio_id"
	mediaAudioExpiresAtKey  = "openai/expires_at"
	mediaAudioDataKey       = "openai/data"
	responseCreatedKey      = "openai/created"
	responseServiceTierKey  = "openai/service_tier"
	responseUsageKey        = "openai/usage"
	reasoningContentJSONKey = "reasoning_content"
)

// ChatConfig configures the provider-neutral Core chat adapter. DefaultOptions
// are copied during construction; callers may supply the model per Request.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

// Validate verifies construction-time configuration without requiring a
// default model, because Request.Options may select it per call.
func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("openai: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements the minimal Core Model and optional Streamer capabilities.
type Chat struct {
	api      *API
	defaults corechat.Options
}

// NewChat constructs a Core chat adapter.
func NewChat(cfg ChatConfig) (*Chat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cloneProtocolOptions(cfg.DefaultOptions)}, nil
}

// Call performs one non-streaming Chat Completions request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.ChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapCompletion(params, response)
}

// Stream performs one streaming Chat Completions request. Each yielded Core
// response represents only the current provider delta; stable tool identity is
// retained in adapter-local state.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := c.buildRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		stream, err := c.api.ChatCompletionStream(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		state := newOpenAIStreamState()
		for stream.Next() {
			response, mapErr := state.mapChunk(stream.Current())
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, streamErr)
		}
	}
}

func (c *Chat) buildRequest(req *corechat.Request) (*openaisdk.ChatCompletionNewParams, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("openai: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}

	params, found, err := metadata.Decode[openaisdk.ChatCompletionNewParams](req.Extensions, requestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("openai: extension %q: %w", requestExtensionKey, err)
	}
	if !found {
		params = openaisdk.ChatCompletionNewParams{}
	}

	options := mergeProtocolOptions(c.defaults, req.Options)
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
		params.MaxCompletionTokens = openaisdk.Int(*options.MaxTokens)
	}
	if options.PresencePenalty != nil {
		params.PresencePenalty = openaisdk.Float(*options.PresencePenalty)
	}
	if len(options.Stop) > 0 {
		params.Stop.OfStringArray = slices.Clone(options.Stop)
	}
	if options.Temperature != nil {
		params.Temperature = openaisdk.Float(*options.Temperature)
	}
	if options.TopP != nil {
		params.TopP = openaisdk.Float(*options.TopP)
	}

	params.Messages, err = mapRequestMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	params.Tools, err = mapToolDefinitions(req.Tools)
	if err != nil {
		return nil, err
	}
	return &params, nil
}

func cloneProtocolOptions(options corechat.Options) corechat.Options {
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

func mapRequestMessages(messages []corechat.Message) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	result := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for i := range messages {
		mapped, err := mapRequestMessage(messages[i])
		if err != nil {
			return nil, fmt.Errorf("openai: messages[%d]: %w", i, err)
		}
		result = append(result, mapped...)
	}
	return result, nil
}

func mapRequestMessage(message corechat.Message) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	switch message.Role {
	case corechat.RoleSystem:
		return []openaisdk.ChatCompletionMessageParamUnion{openaisdk.SystemMessage(message.Text())}, nil
	case corechat.RoleUser:
		mapped, err := mapUserMessage(message)
		return []openaisdk.ChatCompletionMessageParamUnion{mapped}, err
	case corechat.RoleAssistant:
		mapped, err := mapAssistantMessage(message)
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

func mapAssistantMessage(message corechat.Message) (openaisdk.ChatCompletionMessageParamUnion, error) {
	mapped := openaisdk.AssistantMessage(message.Text())
	assistant := mapped.OfAssistant
	audioID, found, err := metadata.Decode[string](message.Metadata, messageAudioIDKey)
	if err != nil {
		return openaisdk.ChatCompletionMessageParamUnion{}, err
	}
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
			return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: reasoning replay is unsupported", i)
		case corechat.PartMedia:
			if audioID == "" || part.Media.Source.Kind != media.SourceReference || part.Media.Source.Ref != audioID {
				return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: assistant media replay requires matching %q metadata", i, messageAudioIDKey)
			}
		default:
			return openaisdk.ChatCompletionMessageParamUnion{}, fmt.Errorf("parts[%d]: unsupported assistant part %q", i, part.Kind)
		}
	}

	if found && audioID != "" {
		assistant.Audio.ID = audioID
	}
	refusal, found, err := metadata.Decode[string](message.Metadata, messageRefusalKey)
	if err != nil {
		return openaisdk.ChatCompletionMessageParamUnion{}, err
	}
	if found {
		assistant.Refusal = openaisdk.String(refusal)
	}
	return mapped, nil
}

func mapCompletion(params *openaisdk.ChatCompletionNewParams, response *openaisdk.ChatCompletion) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("openai: nil response")
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("openai: response has no choices")
	}
	mapped := &corechat.Response{
		ID:      response.ID,
		Model:   response.Model,
		Choices: make([]corechat.Choice, 0, len(response.Choices)),
		Usage:   mapUsage(response.Usage),
	}
	for i := range response.Choices {
		choice, err := mapCompletionChoice(params, response.Choices[i])
		if err != nil {
			return nil, fmt.Errorf("openai: choices[%d]: %w", i, err)
		}
		mapped.Choices = append(mapped.Choices, choice)
	}
	if err := setResponseMetadata(mapped, response.Created, string(response.ServiceTier), response.Usage); err != nil {
		return nil, err
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai: mapped response: %w", err)
	}
	return mapped, nil
}

func mapCompletionChoice(params *openaisdk.ChatCompletionNewParams, choice openaisdk.ChatCompletionChoice) (corechat.Choice, error) {
	mapped := corechat.Choice{
		Index:        int(choice.Index),
		FinishReason: normalizeFinishReason(choice.FinishReason),
	}
	if err := mapped.SetExtension(nativeFinishReasonKey, choice.FinishReason); err != nil {
		return corechat.Choice{}, err
	}
	if choice.JSON.Logprobs.Valid() || len(choice.Logprobs.Content) > 0 || len(choice.Logprobs.Refusal) > 0 {
		if err := mapped.SetExtension(choiceLogprobsKey, choice.Logprobs); err != nil {
			return corechat.Choice{}, err
		}
	}
	message, err := mapCompletionMessage(params, choice.Message)
	if err != nil {
		return corechat.Choice{}, err
	}
	mapped.Message = message
	return mapped, nil
}

func mapCompletionMessage(params *openaisdk.ChatCompletionNewParams, message openaisdk.ChatCompletionMessage) (*corechat.Message, error) {
	parts := make([]corechat.Part, 0, 3+len(message.ToolCalls))
	if reasoning, ok, err := extraString(message.JSON.ExtraFields, reasoningContentJSONKey); err != nil {
		return nil, err
	} else if ok && reasoning != "" {
		parts = append(parts, corechat.NewReasoningPart(reasoning, nil))
	}
	if message.Content != "" {
		parts = append(parts, corechat.NewTextPart(message.Content))
	}
	for i := range message.ToolCalls {
		call, err := mapResponseToolCall(message.ToolCalls[i])
		if err != nil {
			return nil, fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
		parts = append(parts, corechat.NewToolCallPart(call))
	}
	if message.Audio.ID != "" || message.Audio.Data != "" {
		audio, err := mapOutputAudio(params, message.Audio)
		if err != nil {
			return nil, err
		}
		if message.Audio.Transcript != "" && message.Content == "" {
			parts = append(parts, corechat.NewTextPart(message.Audio.Transcript))
		}
		parts = append(parts, corechat.NewMediaPart(audio))
	}
	if len(parts) == 0 {
		return nil, nil
	}
	mapped := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	if err := mapped.Metadata.Set(messageRefusalKey, message.Refusal); err != nil {
		return nil, err
	}
	if message.Audio.ID != "" {
		if err := mapped.Metadata.Set(messageAudioIDKey, message.Audio.ID); err != nil {
			return nil, err
		}
	}
	if len(message.Annotations) > 0 {
		if err := mapped.Metadata.Set(messageAnnotationsKey, message.Annotations); err != nil {
			return nil, err
		}
	}
	return mapped, nil
}

func mapResponseToolCall(toolCall openaisdk.ChatCompletionMessageToolCallUnion) (corechat.ToolCall, error) {
	switch toolCall.Type {
	case "", "function":
		return corechat.ToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		}, nil
	case "custom":
		return corechat.ToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Custom.Name,
			Arguments: toolCall.Custom.Input,
		}, nil
	default:
		return corechat.ToolCall{}, fmt.Errorf("unsupported type %q", toolCall.Type)
	}
}

func mapOutputAudio(params *openaisdk.ChatCompletionNewParams, audio openaisdk.ChatCompletionAudio) (*media.Media, error) {
	mimeType := audioMIME(string(params.Audio.Format))
	var mapped *media.Media
	var err error
	if audio.ID != "" {
		mapped, err = media.NewReference(mimeType, audio.ID)
	} else {
		data, decodeErr := base64.StdEncoding.DecodeString(audio.Data)
		if decodeErr != nil {
			return nil, fmt.Errorf("openai: decode output audio: %w", decodeErr)
		}
		mapped, err = media.NewBytes(mimeType, data)
	}
	if err != nil {
		return nil, err
	}
	mapped.ID = audio.ID
	if audio.ExpiresAt != 0 {
		if err := mapped.Metadata.Set(mediaAudioExpiresAtKey, audio.ExpiresAt); err != nil {
			return nil, err
		}
	}
	if audio.Data != "" {
		if err := mapped.Metadata.Set(mediaAudioDataKey, audio.Data); err != nil {
			return nil, err
		}
	}
	return mapped, nil
}

func audioMIME(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/opus"
	case "aac":
		return "audio/aac"
	case "pcm16":
		return "audio/L16"
	default:
		return "audio/octet-stream"
	}
}

func mapUsage(usage openaisdk.CompletionUsage) corechat.Usage {
	mapped := corechat.Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
	if usage.CompletionTokensDetails.JSON.ReasoningTokens.Valid() || usage.CompletionTokensDetails.ReasoningTokens != 0 {
		value := usage.CompletionTokensDetails.ReasoningTokens
		mapped.ReasoningTokens = &value
	}
	if usage.PromptTokensDetails.JSON.CachedTokens.Valid() || usage.PromptTokensDetails.CachedTokens != 0 {
		value := usage.PromptTokensDetails.CachedTokens
		mapped.CacheReadInputTokens = &value
	}
	return mapped
}

func setResponseMetadata(response *corechat.Response, created int64, serviceTier string, usage openaisdk.CompletionUsage) error {
	if created != 0 {
		if err := response.SetExtension(responseCreatedKey, created); err != nil {
			return err
		}
	}
	if serviceTier != "" {
		if err := response.SetExtension(responseServiceTierKey, serviceTier); err != nil {
			return err
		}
	}
	if err := response.SetExtension(responseUsageKey, usage); err != nil {
		return err
	}
	return nil
}

func normalizeFinishReason(reason string) corechat.FinishReason {
	switch reason {
	case "":
		return ""
	case "stop":
		return corechat.FinishReasonStop
	case "length":
		return corechat.FinishReasonLength
	case "tool_calls", "function_call":
		return corechat.FinishReasonToolCalls
	case "content_filter":
		return corechat.FinishReasonContentFilter
	default:
		return corechat.FinishReasonOther
	}
}

func extraString(fields map[string]respjson.Field, key string) (string, bool, error) {
	field, ok := fields[key]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal([]byte(field.Raw()), &value); err != nil {
		return "", true, fmt.Errorf("openai: decode %s: %w", key, err)
	}
	return value, true, nil
}

type openAIStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type openAIStreamState struct {
	tools map[int]map[int64]openAIStreamTool
}

func newOpenAIStreamState() *openAIStreamState {
	return &openAIStreamState{tools: make(map[int]map[int64]openAIStreamTool)}
}

func (s *openAIStreamState) mapChunk(chunk openaisdk.ChatCompletionChunk) (*corechat.Response, error) {
	mapped := &corechat.Response{
		ID:      chunk.ID,
		Model:   chunk.Model,
		Choices: make([]corechat.Choice, 0, len(chunk.Choices)),
		Usage:   mapUsage(chunk.Usage),
	}
	for i := range chunk.Choices {
		choice, include, err := s.mapChunkChoice(chunk.Choices[i])
		if err != nil {
			return nil, fmt.Errorf("openai: stream choices[%d]: %w", i, err)
		}
		if include {
			mapped.Choices = append(mapped.Choices, choice)
		}
	}
	if err := setResponseMetadata(mapped, chunk.Created, string(chunk.ServiceTier), chunk.Usage); err != nil {
		return nil, err
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai: mapped stream response: %w", err)
	}
	return mapped, nil
}

func (s *openAIStreamState) mapChunkChoice(choice openaisdk.ChatCompletionChunkChoice) (corechat.Choice, bool, error) {
	index := int(choice.Index)
	mapped := corechat.Choice{Index: index, FinishReason: normalizeFinishReason(choice.FinishReason)}
	if choice.FinishReason != "" {
		if err := mapped.SetExtension(nativeFinishReasonKey, choice.FinishReason); err != nil {
			return corechat.Choice{}, false, err
		}
	}
	if choice.JSON.Logprobs.Valid() || len(choice.Logprobs.Content) > 0 || len(choice.Logprobs.Refusal) > 0 {
		if err := mapped.SetExtension(choiceLogprobsKey, choice.Logprobs); err != nil {
			return corechat.Choice{}, false, err
		}
	}

	parts := make([]corechat.Part, 0, 2+len(choice.Delta.ToolCalls))
	if reasoning, ok, err := extraString(choice.Delta.JSON.ExtraFields, reasoningContentJSONKey); err != nil {
		return corechat.Choice{}, false, err
	} else if ok && reasoning != "" {
		parts = append(parts, corechat.NewReasoningPart(reasoning, nil))
	}
	if choice.Delta.Content != "" {
		parts = append(parts, corechat.NewTextPart(choice.Delta.Content))
	}
	for i := range choice.Delta.ToolCalls {
		call, include, err := s.mapChunkTool(index, choice.Delta.ToolCalls[i])
		if err != nil {
			return corechat.Choice{}, false, fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
		if include {
			parts = append(parts, corechat.NewToolCallPart(call))
		}
	}
	if len(parts) > 0 {
		message := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
		if choice.Delta.Refusal != "" {
			if err := message.Metadata.Set(messageRefusalKey, choice.Delta.Refusal); err != nil {
				return corechat.Choice{}, false, err
			}
		}
		mapped.Message = message
	} else if choice.Delta.Refusal != "" {
		if err := mapped.SetExtension(choiceRefusalDeltaKey, choice.Delta.Refusal); err != nil {
			return corechat.Choice{}, false, err
		}
	}

	include := mapped.Message != nil || mapped.FinishReason != "" || len(mapped.Extensions) > 0
	return mapped, include, nil
}

func (s *openAIStreamState) mapChunkTool(choiceIndex int, delta openaisdk.ChatCompletionChunkChoiceDeltaToolCall) (corechat.ToolCall, bool, error) {
	if delta.Type != "" && delta.Type != "function" {
		return corechat.ToolCall{}, false, fmt.Errorf("unsupported type %q", delta.Type)
	}
	choiceTools := s.tools[choiceIndex]
	if choiceTools == nil {
		choiceTools = make(map[int64]openAIStreamTool)
		s.tools[choiceIndex] = choiceTools
	}
	state := choiceTools[delta.Index]
	if delta.ID != "" {
		state.id = delta.ID
	}
	if delta.Function.Name != "" {
		state.name = delta.Function.Name
	}
	state.pendingArguments += delta.Function.Arguments
	choiceTools[delta.Index] = state
	if state.id == "" || state.name == "" {
		return corechat.ToolCall{}, false, nil
	}
	arguments := state.pendingArguments
	state.pendingArguments = ""
	choiceTools[delta.Index] = state
	return corechat.ToolCall{
		ID:        state.id,
		Name:      state.name,
		Arguments: arguments,
	}, true, nil
}
