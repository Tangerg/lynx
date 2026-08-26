package mistral

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"mime"
	"net/http"
	"slices"
	"strings"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

const (
	RequestExtensionKey     = "mistral/request"
	responseExtensionKey    = "mistral/response"
	streamChunkExtensionKey = "mistral/chunk"
)

// ReasoningEffort controls Mistral's native reasoning mode.
type ReasoningEffort string

const (
	ReasoningEffortHigh ReasoningEffort = "high"
	ReasoningEffortNone ReasoningEffort = "none"
)

func (effort ReasoningEffort) Validate() error {
	switch effort {
	case "", ReasoningEffortHigh, ReasoningEffortNone:
		return nil
	default:
		return fmt.Errorf("unsupported reasoning effort %q", effort)
	}
}

// ChatRequestOptions exposes Mistral-specific Chat Completions parameters that
// have no provider-neutral Core equivalent. Store it under RequestExtensionKey.
type ChatRequestOptions struct {
	ReasoningEffort   ReasoningEffort   `json:"reasoning_effort,omitempty"`
	RandomSeed        *int64            `json:"random_seed,omitempty"`
	SafePrompt        *bool             `json:"safe_prompt,omitempty"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls,omitempty"`
	PromptCacheKey    string            `json:"prompt_cache_key,omitempty"`
	ToolChoice        json.RawMessage   `json:"tool_choice,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
	Guardrails        []json.RawMessage `json:"guardrails,omitempty"`
}

func (options ChatRequestOptions) Validate() error {
	if err := options.ReasoningEffort.Validate(); err != nil {
		return err
	}
	for name, raw := range map[string]json.RawMessage{
		"tool_choice": options.ToolChoice,
	} {
		if len(raw) > 0 && !json.Valid(raw) {
			return fmt.Errorf("%s contains invalid JSON", name)
		}
	}
	for index := range options.Guardrails {
		if !json.Valid(options.Guardrails[index]) {
			return fmt.Errorf("guardrails[%d] contains invalid JSON", index)
		}
	}
	return nil
}

// ChatConfig configures Mistral's native Chat Completions adapter.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config ChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("mistral: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("mistral: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Mistral's native Chat Completions protocol, including
// structured thinking chunks and their multi-turn replay semantics.
type Chat struct {
	api      *api
	defaults corechat.Options
}

// NewChat constructs a Mistral Chat Completions adapter.
func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    cmp.Or(config.BaseURL, DefaultBaseURL),
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: config.DefaultOptions.Clone()}, nil
}

func (chat *Chat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	wireRequest, err := chat.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	wireResponse, err := chat.api.chatCompletion(ctx, wireRequest)
	if err != nil {
		return nil, err
	}
	return mapChatCompletion(wireResponse)
}

func (chat *Chat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		wireRequest, err := chat.buildRequest(request, true)
		if err != nil {
			yield(nil, err)
			return
		}
		body, err := chat.api.chatCompletionStream(ctx, wireRequest)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		state := newChatStreamState()
		if err := scanMistralSSE(body, func(data []byte) bool {
			var chunk chatCompletionChunk
			if decodeErr := json.Unmarshal(data, &chunk); decodeErr != nil {
				err = fmt.Errorf("mistral: decode chat stream chunk: %w", decodeErr)
				return false
			}
			response, mapErr := state.mapChunk(chunk)
			if mapErr != nil {
				err = mapErr
				return false
			}
			return yield(response, nil)
		}); err != nil {
			yield(nil, err)
		}
	}
}

func (chat *Chat) buildRequest(request *corechat.Request, stream bool) (*chatCompletionRequest, error) {
	if chat == nil || chat.api == nil {
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
		if err := extension.Validate(); err != nil {
			return nil, fmt.Errorf("mistral: extension %q: %w", RequestExtensionKey, err)
		}
	}
	options, err := chat.defaults.Merged(request.Options)
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

func mapChatCompletion(completion *chatCompletionResponse) (*corechat.Response, error) {
	if completion == nil {
		return nil, errors.New("mistral: nil chat completion response")
	}
	if len(completion.Choices) != 1 {
		return nil, fmt.Errorf("mistral: response has %d choices; Core requires one output", len(completion.Choices))
	}
	response := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			ID: completion.ID, Model: completion.Model, Usage: mapMistralUsage(completion.Usage),
		},
	}
	if err := response.Metadata.Set(responseExtensionKey, completion); err != nil {
		return nil, err
	}
	wireChoice := completion.Choices[0]
	if wireChoice.Index != 0 {
		return nil, fmt.Errorf("mistral: choice index is %d, want 0", wireChoice.Index)
	}
	parts, err := mapMistralContent(wireChoice.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("mistral: output message content: %w", err)
	}
	toolParts, err := mapMistralToolCalls(wireChoice.Message.ToolCalls)
	if err != nil {
		return nil, fmt.Errorf("mistral: output message tool calls: %w", err)
	}
	parts = append(parts, toolParts...)
	response.Output = &corechat.Output{FinishReason: normalizeMistralFinishReason(wireChoice.FinishReason)}
	if len(parts) > 0 {
		response.Output.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("mistral: mapped chat completion: %w", err)
	}
	return response, nil
}

func mapMistralContent(raw json.RawMessage) ([]corechat.Part, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return []corechat.Part{corechat.NewTextPart(text)}, nil
	}
	var chunks []json.RawMessage
	if err := json.Unmarshal(trimmed, &chunks); err != nil {
		return nil, err
	}
	parts := make([]corechat.Part, 0, len(chunks))
	for index := range chunks {
		var discriminator struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(chunks[index], &discriminator); err != nil {
			return nil, fmt.Errorf("chunk[%d]: %w", index, err)
		}
		switch discriminator.Type {
		case "text":
			var chunk textChunk
			if err := json.Unmarshal(chunks[index], &chunk); err != nil {
				return nil, fmt.Errorf("chunk[%d]: %w", index, err)
			}
			if chunk.Text != "" {
				parts = append(parts, corechat.NewTextPart(chunk.Text))
			}
		case "thinking":
			var chunk struct {
				Thinking []json.RawMessage `json:"thinking"`
			}
			if err := json.Unmarshal(chunks[index], &chunk); err != nil {
				return nil, fmt.Errorf("chunk[%d]: %w", index, err)
			}
			var reasoning strings.Builder
			for nestedIndex := range chunk.Thinking {
				var nested textChunk
				if err := json.Unmarshal(chunk.Thinking[nestedIndex], &nested); err == nil && nested.Type == "text" {
					reasoning.WriteString(nested.Text)
				}
			}
			frame, err := encodeThinkingFrame(chunks[index])
			if err != nil {
				return nil, fmt.Errorf("chunk[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewReasoningPart(reasoning.String(), frame))
		case "image_url":
			var chunk imageURLChunk
			if err := json.Unmarshal(chunks[index], &chunk); err != nil {
				return nil, fmt.Errorf("chunk[%d]: %w", index, err)
			}
			image, err := media.NewURI("image/*", string(chunk.ImageURL))
			if err != nil {
				return nil, fmt.Errorf("chunk[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(image))
		case "reference", "tool_reference":
			// Reference chunks remain available losslessly in the response or
			// stream-chunk extension. Core has no citation part kind.
			continue
		default:
			return nil, fmt.Errorf("chunk[%d]: unsupported content type %q", index, discriminator.Type)
		}
	}
	return parts, nil
}

func mapMistralToolCalls(calls []chatToolCall) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(calls))
	for index := range calls {
		call := calls[index]
		if call.ID == "" {
			return nil, fmt.Errorf("tool call %d has no ID", index)
		}
		if call.Function.Name == "" {
			return nil, fmt.Errorf("tool call %d has no function name", index)
		}
		arguments, err := mistralToolArguments(call.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
		}
		parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
			ID: call.ID, Name: call.Function.Name, Arguments: arguments,
		}))
	}
	return parts, nil
}

func mistralToolArguments(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	if !json.Valid(trimmed) {
		return "", errors.New("invalid JSON")
	}
	return string(trimmed), nil
}

func mapMistralUsage(usage chatUsage) corechat.Usage {
	mapped := corechat.Usage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens}
	cached := usage.NumCachedTokens
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens != 0 {
		cached = usage.PromptTokensDetails.CachedTokens
	}
	if cached != 0 {
		mapped.CacheReadInputTokens = &cached
	}
	return mapped
}

func normalizeMistralFinishReason(reason string) corechat.FinishReason {
	switch reason {
	case "":
		return ""
	case "stop":
		return corechat.FinishReasonStop
	case "length", "model_length":
		return corechat.FinishReasonLength
	case "tool_calls":
		return corechat.FinishReasonToolCalls
	default:
		return corechat.FinishReasonOther
	}
}

type chatStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type chatStreamState struct {
	tools map[int]chatStreamTool
}

func newChatStreamState() *chatStreamState {
	return &chatStreamState{tools: make(map[int]chatStreamTool)}
}

func (state *chatStreamState) mapChunk(chunk chatCompletionChunk) (*corechat.Response, error) {
	response := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			ID: chunk.ID, Model: chunk.Model, Usage: mapMistralUsage(chunk.Usage),
		},
	}
	if err := response.Metadata.Set(streamChunkExtensionKey, chunk); err != nil {
		return nil, err
	}
	if len(chunk.Choices) > 1 {
		return nil, fmt.Errorf("mistral: stream chunk has %d choices; Core supports one output", len(chunk.Choices))
	}
	if len(chunk.Choices) == 1 {
		wireChoice := chunk.Choices[0]
		if wireChoice.Index != 0 {
			return nil, fmt.Errorf("mistral: stream choice index is %d, want 0", wireChoice.Index)
		}
		parts, err := mapMistralContent(wireChoice.Delta.Content)
		if err != nil {
			return nil, fmt.Errorf("mistral: stream output content: %w", err)
		}
		toolParts, err := state.mapToolDeltas(wireChoice.Delta.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("mistral: stream output tool calls: %w", err)
		}
		parts = append(parts, toolParts...)
		response.Output = &corechat.Output{
			FinishReason: normalizeMistralFinishReason(wireChoice.FinishReason),
		}
		if len(parts) > 0 {
			response.Output.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
		}
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("mistral: mapped stream chunk: %w", err)
	}
	return response, nil
}

func (state *chatStreamState) mapToolDeltas(calls []chatToolCall) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(calls))
	for position := range calls {
		call := calls[position]
		index := call.Index
		tool := state.tools[index]
		if call.ID != "" {
			tool.id = call.ID
		}
		if call.Function.Name != "" {
			tool.name = call.Function.Name
		}
		arguments, err := mistralToolArguments(call.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool call %d arguments: %w", index, err)
		}
		tool.pendingArguments += arguments
		state.tools[index] = tool
		if tool.id == "" || tool.name == "" {
			continue
		}
		deltaArguments := tool.pendingArguments
		tool.pendingArguments = ""
		state.tools[index] = tool
		parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
			ID: tool.id, Name: tool.name, Arguments: deltaArguments,
		}))
	}
	return parts, nil
}

func scanMistralSSE(reader io.Reader, yield func([]byte) bool) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var data []byte
	flush := func() bool {
		if len(data) == 0 {
			return true
		}
		payload := bytes.TrimSpace(data)
		data = data[:0]
		if bytes.Equal(payload, []byte("[DONE]")) {
			return false
		}
		return yield(payload)
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if !flush() {
				return nil
			}
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimSpace(line[len("data:"):])...)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mistral: read chat stream: %w", err)
	}
	flush()
	return nil
}
