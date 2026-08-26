package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

const (
	// RequestExtensionKey identifies provider-owned Chat Completions fields
	// encoded as [RequestFields].
	RequestExtensionKey = "openai/request"
	// ResponseExtensionKey preserves the complete official Chat Completions
	// response after provider-neutral fields have been mapped.
	ResponseExtensionKey = "openai/response"
	// StreamChunkExtensionKey preserves each complete official Chat
	// Completions stream chunk.
	StreamChunkExtensionKey = "openai/stream_chunk"
)

// ChatConfig configures an OpenAI Chat Completions adapter. DefaultOptions
// are copied during construction; callers may select the model per request.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
	Headers        http.Header
}

// Validate verifies construction-time configuration.
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

// Chat implements OpenAI's Chat Completions protocol and is also the reusable
// protocol base for provider packages exposing a compatible endpoint.
type Chat struct {
	api      *api
	defaults corechat.Options
	dialect  Dialect
}

// NewChat constructs OpenAI's native Chat Completions adapter.
func NewChat(config ChatConfig) (*Chat, error) {
	return newChat(config, Dialect{Provider: "openai", TokenLimitField: TokenLimitMaxCompletionTokens})
}

// NewCompatibleChat constructs a Chat Completions adapter with one explicit
// provider dialect. Provider packages use this seam; application code should
// prefer the provider's own constructor.
func NewCompatibleChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	return newChat(config, dialect)
}

func newChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := dialect.Validate(); err != nil {
		return nil, fmt.Errorf("openai: dialect: %w", err)
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
		Headers:    config.Headers,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{
		api:      api,
		defaults: config.DefaultOptions.Clone(),
		dialect:  dialect,
	}, nil
}

// Call performs one non-streaming Chat Completions request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildRequest(req, false)
	if err != nil {
		return nil, err
	}
	response, err := c.api.chatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapCompletion(params, response, c.dialect)
}

// Stream performs one streaming Chat Completions request. Each yielded Core
// response represents only the current provider delta; stable tool identity is
// retained in adapter-local state.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := c.buildRequest(req, true)
		if err != nil {
			yield(nil, err)
			return
		}

		stream, err := c.api.chatCompletionStream(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		state := newOpenAIStreamState(c.dialect)
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
			yield(nil, c.api.wrapError(streamErr))
		}
	}
}

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
			"max_completion_tokens", "presence_penalty", "response_format", "stop", "temperature", "top_p",
		)
		if err != nil {
			return nil, err
		}
		if _, exists := fields["n"]; exists {
			return nil, fmt.Errorf("openai: extension %q field %q is unsupported; Core Chat produces one output", extensionKey, "n")
		}
		params.SetExtraFields(fields)
	}

	options, err := c.defaults.Merged(req.Options)
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

func mapCompletion(params *openaisdk.ChatCompletionNewParams, response *openaisdk.ChatCompletion, dialect Dialect) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("openai: nil response")
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("openai: response has no choices")
	}
	if len(response.Choices) > 1 {
		return nil, fmt.Errorf("openai: response has %d choices; Core supports one output", len(response.Choices))
	}
	output, err := mapCompletionOutput(params, response.Choices[0], dialect.Provider, dialect.response)
	if err != nil {
		return nil, fmt.Errorf("openai: output: %w", err)
	}
	mapped := &corechat.Response{
		Output: output,
		Metadata: &corechat.ResponseMetadata{
			ID:    response.ID,
			Model: response.Model,
			Usage: mapUsage(response.Usage),
		},
	}
	if err := mapped.Metadata.Set(protocolResponseExtensionKey(dialect.Provider), exactProviderResponse(response.RawJSON(), response)); err != nil {
		return nil, err
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai: mapped response: %w", err)
	}
	return mapped, nil
}

func mapCompletionOutput(params *openaisdk.ChatCompletionNewParams, choice openaisdk.ChatCompletionChoice, provider string, dialect responseDialect) (*corechat.Output, error) {
	if choice.Index != 0 {
		return nil, fmt.Errorf("choice index is %d, want 0", choice.Index)
	}
	mapped := &corechat.Output{FinishReason: normalizeFinishReason(choice.FinishReason)}
	message, err := mapCompletionMessage(params, choice.Message, provider, dialect)
	if err != nil {
		return nil, err
	}
	mapped.Message = message
	return mapped, nil
}

func mapCompletionMessage(params *openaisdk.ChatCompletionNewParams, message openaisdk.ChatCompletionMessage, provider string, dialect responseDialect) (*corechat.Message, error) {
	parts := make([]corechat.Part, 0, 3+len(message.ToolCalls))
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
	mapped := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	if dialect != nil {
		if err := dialect.FinalizeMessage(message, mapped); err != nil {
			return nil, fmt.Errorf("response dialect: %w", err)
		}
	}
	if len(mapped.Parts) == 0 {
		return nil, nil
	}
	if message.Refusal != "" {
		if err := mapped.Metadata.Set(protocolRefusalExtensionKey(provider), message.Refusal); err != nil {
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
	format := string(params.Audio.Format)
	if format == "" {
		if audioField, ok := params.ExtraFields()["audio"].(map[string]any); ok {
			format, _ = audioField["format"].(string)
		}
	}
	mimeType := audioMIME(format)
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

type openAIStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type openAIStreamState struct {
	tools       map[int64]openAIStreamTool
	dialect     responseDialect
	chunkKey    string
	refusalKey  string
	refusalPart string
}

func newOpenAIStreamState(dialect Dialect) *openAIStreamState {
	return &openAIStreamState{
		tools:       make(map[int64]openAIStreamTool),
		dialect:     dialect.response,
		chunkKey:    protocolStreamChunkExtensionKey(dialect.Provider),
		refusalKey:  protocolRefusalDeltaExtensionKey(dialect.Provider),
		refusalPart: protocolRefusalExtensionKey(dialect.Provider),
	}
}

func (o *openAIStreamState) mapChunk(chunk openaisdk.ChatCompletionChunk) (*corechat.Response, error) {
	mapped := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			ID:    chunk.ID,
			Model: chunk.Model,
			Usage: mapUsage(chunk.Usage),
		},
	}
	if len(chunk.Choices) > 1 {
		return nil, fmt.Errorf("openai: stream chunk has %d choices; Core supports one output", len(chunk.Choices))
	}
	if len(chunk.Choices) == 1 {
		output, include, err := o.mapChunkOutput(chunk.Choices[0])
		if err != nil {
			return nil, fmt.Errorf("openai: stream output: %w", err)
		}
		if include {
			mapped.Output = output
		}
	}
	if err := mapped.Metadata.Set(o.chunkKey, exactProviderResponse(chunk.RawJSON(), chunk)); err != nil {
		return nil, err
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai: mapped stream response: %w", err)
	}
	return mapped, nil
}

func exactProviderResponse(raw string, fallback any) any {
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return fallback
}

func (o *openAIStreamState) mapChunkOutput(choice openaisdk.ChatCompletionChunkChoice) (*corechat.Output, bool, error) {
	if choice.Index != 0 {
		return nil, false, fmt.Errorf("choice index is %d, want 0", choice.Index)
	}
	mapped := &corechat.Output{FinishReason: normalizeFinishReason(choice.FinishReason)}
	parts := make([]corechat.Part, 0, 2+len(choice.Delta.ToolCalls))
	if choice.Delta.Content != "" {
		parts = append(parts, corechat.NewTextPart(choice.Delta.Content))
	}
	for i := range choice.Delta.ToolCalls {
		call, include, err := o.mapChunkTool(choice.Delta.ToolCalls[i])
		if err != nil {
			return nil, false, fmt.Errorf("tool_calls[%d]: %w", i, err)
		}
		if include {
			parts = append(parts, corechat.NewToolCallPart(call))
		}
	}
	message := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	if o.dialect != nil {
		if err := o.dialect.FinalizeDelta(choice.Delta, message); err != nil {
			return nil, false, fmt.Errorf("response dialect: %w", err)
		}
	}
	if len(message.Parts) > 0 {
		if choice.Delta.Refusal != "" {
			if err := message.Metadata.Set(o.refusalPart, choice.Delta.Refusal); err != nil {
				return nil, false, err
			}
		}
		mapped.Message = message
	} else if choice.Delta.Refusal != "" {
		mapped.Metadata = &corechat.OutputMetadata{}
		if err := mapped.Metadata.Set(o.refusalKey, choice.Delta.Refusal); err != nil {
			return nil, false, err
		}
	}

	include := mapped.Message != nil || mapped.FinishReason != "" || mapped.Metadata != nil
	return mapped, include, nil
}

func (o *openAIStreamState) mapChunkTool(delta openaisdk.ChatCompletionChunkChoiceDeltaToolCall) (corechat.ToolCall, bool, error) {
	if delta.Type != "" && delta.Type != "function" {
		return corechat.ToolCall{}, false, fmt.Errorf("unsupported type %q", delta.Type)
	}
	state := o.tools[delta.Index]
	if delta.ID != "" {
		state.id = delta.ID
	}
	if delta.Function.Name != "" {
		state.name = delta.Function.Name
	}
	state.pendingArguments += delta.Function.Arguments
	o.tools[delta.Index] = state
	if state.id == "" || state.name == "" {
		return corechat.ToolCall{}, false, nil
	}
	arguments := state.pendingArguments
	state.pendingArguments = ""
	o.tools[delta.Index] = state
	return corechat.ToolCall{
		ID:        state.id,
		Name:      state.name,
		Arguments: arguments,
	}, true, nil
}
