package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const (
	reasoningContentField    = "reasoning_content"
	reasoningField           = "reasoning"
	textReasoningProviderKey = "openai/reasoning_provider"
	textReasoningFieldKey    = "openai/reasoning_field"
)

// TextReasoningField identifies a provider's plain-text reasoning property.
type TextReasoningField string

const (
	TextReasoningContent TextReasoningField = reasoningContentField
	TextReasoning        TextReasoningField = reasoningField
)

type requestDialect interface {
	PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error
}

type responseDialect interface {
	FinalizeMessage(source openaisdk.ChatCompletionMessage, target *corechat.Message) error
	FinalizeDelta(source openaisdk.ChatCompletionChunkChoiceDelta, target *corechat.Message) error
}

// CompatibleRequest exposes the stable subset of a compatible request that a
// provider dialect may inspect. Provider-only top-level JSON fields are added
// with SetExtraField; OpenAI SDK wire types never cross this boundary.
type CompatibleRequest struct {
	model       string
	temperature *float64
	stream      bool
	extraFields map[string]any
}

// Model returns the effective model after Core defaults and request options
// have been merged.
func (request *CompatibleRequest) Model() string { return request.model }

// Temperature returns the effective temperature when one was supplied.
func (request *CompatibleRequest) Temperature() (float64, bool) {
	if request.temperature == nil {
		return 0, false
	}
	return *request.temperature, true
}

// Stream reports whether the request will use the streaming endpoint.
func (request *CompatibleRequest) Stream() bool { return request.stream }

// SetExtraField adds or replaces one provider-owned top-level JSON field.
func (request *CompatibleRequest) SetExtraField(name string, value any) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
		return errors.New("openai: extra field name is required and must not have surrounding whitespace")
	}
	if request.extraFields == nil {
		request.extraFields = make(map[string]any)
	}
	request.extraFields[name] = value
	return nil
}

// Dialect groups the independently typed request and response protocol facets
// selected by a provider adapter. One Chat has exactly one dialect.
type Dialect struct {
	// Provider scopes provider-owned response and replay state.
	Provider        string
	PrepareRequest  func(source *corechat.Request, target *CompatibleRequest) error
	TokenLimitField TokenLimitField
	// DisableRawRequestExtension prevents provider adapters from accepting an
	// arbitrary OpenAI request object when their documented request surface is
	// narrower than OpenAI's.
	DisableRawRequestExtension bool

	request  requestDialect
	response responseDialect
}

// TokenLimitField identifies the provider's wire field for Core's neutral
// Options.MaxTokens value. OpenAI-compatible APIs are not uniform here:
// legacy-compatible providers accept max_tokens while newer protocols use
// max_completion_tokens.
type TokenLimitField uint8

const (
	// TokenLimitMaxTokens selects the legacy-compatible max_tokens field. It is
	// the zero value because most compatibility endpoints still require it.
	TokenLimitMaxTokens TokenLimitField = iota
	// TokenLimitMaxCompletionTokens selects max_completion_tokens.
	TokenLimitMaxCompletionTokens
)

// ReasoningContentDialect maps the common reasoning_content extension while
// treating it as output-only state.
func ReasoningContentDialect(provider string) Dialect {
	codec := textReasoningCodec{provider: provider, field: reasoningContentField}
	return Dialect{Provider: provider, response: codec}
}

// ReasoningContentReplayDialect maps reasoning_content and sends it back on
// every assistant message. Providers select this only when their protocol
// treats historical reasoning as replayable conversation state.
func ReasoningContentReplayDialect(provider string) Dialect {
	codec := textReasoningCodec{provider: provider, field: reasoningContentField, replay: reasoningReplayAlways}
	return Dialect{Provider: provider, request: codec, response: codec}
}

// ReasoningContentToolReplayDialect maps reasoning_content and sends it back
// only on assistant messages containing tool calls.
func ReasoningContentToolReplayDialect(provider string) Dialect {
	codec := textReasoningCodec{provider: provider, field: reasoningContentField, replay: reasoningReplayWithToolCalls}
	return Dialect{Provider: provider, request: codec, response: codec}
}

// ReasoningDialect maps the reasoning extension as output-only state.
func ReasoningDialect(provider string) Dialect {
	codec := textReasoningCodec{provider: provider, field: reasoningField}
	return Dialect{Provider: provider, response: codec}
}

// ReasoningReplayDialect maps the reasoning extension and sends it back on
// every assistant message.
func ReasoningReplayDialect(provider string) Dialect {
	codec := textReasoningCodec{provider: provider, field: reasoningField, replay: reasoningReplayAlways}
	return Dialect{Provider: provider, request: codec, response: codec}
}

type reasoningReplay uint8

const (
	reasoningReplayNever reasoningReplay = iota
	reasoningReplayAlways
	reasoningReplayWithToolCalls
)

type textReasoningCodec struct {
	provider string
	field    string
	replay   reasoningReplay
}

func protocolRequestExtensionKey(provider string) string {
	if provider == "openai" {
		return RequestExtensionKey
	}
	return provider + "/openai_request"
}

func validateProvider(provider string) error {
	if provider == "" || strings.TrimSpace(provider) != provider || strings.Contains(provider, "/") {
		return errors.New("provider is required, must not contain '/', and must not have surrounding whitespace")
	}
	return nil
}

func protocolModalityRequestExtensionKey(provider, modality string) string {
	return provider + "/" + modality + "_request"
}

func protocolResponseExtensionKey(provider string) string {
	if provider == "openai" {
		return ResponseExtensionKey
	}
	return provider + "/openai_response"
}

func protocolStreamChunkExtensionKey(provider string) string {
	if provider == "openai" {
		return StreamChunkExtensionKey
	}
	return provider + "/openai_stream_chunk"
}

func protocolRefusalExtensionKey(provider string) string {
	if provider == "openai" {
		return "openai/refusal"
	}
	return provider + "/openai_refusal"
}

func protocolRefusalDeltaExtensionKey(provider string) string {
	if provider == "openai" {
		return "openai/refusal_delta"
	}
	return provider + "/openai_refusal_delta"
}

func (codec textReasoningCodec) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	wireIndex := 0
	for i := range source.Messages {
		message := source.Messages[i]
		if message.Role == corechat.RoleAssistant {
			if wireIndex >= len(target.Messages) || target.Messages[wireIndex].OfAssistant == nil {
				return fmt.Errorf("messages[%d]: assistant wire mapping is missing", i)
			}
			reasoning, hasToolCalls, err := assistantReplayState(message, codec.provider, codec.field)
			if err != nil {
				return fmt.Errorf("messages[%d]: %w", i, err)
			}
			shouldReplay := codec.replay == reasoningReplayAlways ||
				(codec.replay == reasoningReplayWithToolCalls && hasToolCalls)
			if reasoning != "" && shouldReplay {
				target.Messages[wireIndex].OfAssistant.SetExtraFields(map[string]any{codec.field: reasoning})
			}
		}
		wireIndex += wireMessageCount(message)
	}
	if wireIndex != len(target.Messages) {
		return fmt.Errorf("wire message count = %d; mapped source count = %d", len(target.Messages), wireIndex)
	}
	return nil
}

func (codec textReasoningCodec) FinalizeMessage(source openaisdk.ChatCompletionMessage, target *corechat.Message) error {
	return prependTextReasoning(source.JSON.ExtraFields, codec.provider, codec.field, target)
}

func (codec textReasoningCodec) FinalizeDelta(source openaisdk.ChatCompletionChunkChoiceDelta, target *corechat.Message) error {
	return prependTextReasoning(source.JSON.ExtraFields, codec.provider, codec.field, target)
}

func assistantReplayState(message corechat.Message, provider, field string) (reasoning string, hasToolCalls bool, err error) {
	var builder strings.Builder
	for i := range message.Parts {
		switch message.Parts[i].Kind {
		case corechat.PartReasoning:
			issuer, found, decodeErr := metadata.Decode[string](message.Parts[i].Metadata, textReasoningProviderKey)
			if decodeErr != nil {
				return "", false, fmt.Errorf("parts[%d].metadata[%q]: %w", i, textReasoningProviderKey, decodeErr)
			}
			wireField, fieldFound, decodeErr := metadata.Decode[string](message.Parts[i].Metadata, textReasoningFieldKey)
			if decodeErr != nil {
				return "", false, fmt.Errorf("parts[%d].metadata[%q]: %w", i, textReasoningFieldKey, decodeErr)
			}
			if !found || !fieldFound || issuer != provider || wireField != field {
				continue
			}
			builder.WriteString(message.Parts[i].Text)
		case corechat.PartToolCall:
			hasToolCalls = true
		}
	}
	return builder.String(), hasToolCalls, nil
}

func wireMessageCount(message corechat.Message) int {
	if message.Role == corechat.RoleTool {
		return len(message.Parts)
	}
	return 1
}

func prependTextReasoning(fields map[string]respjson.Field, provider, fieldName string, target *corechat.Message) error {
	field, ok := fields[fieldName]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return nil
	}
	var reasoning string
	if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err != nil {
		return fmt.Errorf("decode %s: %w", fieldName, err)
	}
	if reasoning == "" {
		return nil
	}
	part, err := NewTextReasoningPart(provider, TextReasoningField(fieldName), reasoning)
	if err != nil {
		return err
	}
	target.Parts = append([]corechat.Part{part}, target.Parts...)
	return nil
}

// NewTextReasoningPart creates provider-scoped replay state for manually
// constructed OpenAI-compatible assistant history. Normal callers should keep
// the Part returned by Chat.
func NewTextReasoningPart(provider string, field TextReasoningField, text string) (corechat.Part, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(provider) != provider {
		return corechat.Part{}, errors.New("openai: text reasoning provider is required and must not have surrounding whitespace")
	}
	if field != TextReasoningContent && field != TextReasoning {
		return corechat.Part{}, fmt.Errorf("openai: unsupported text reasoning field %q", field)
	}
	if text == "" {
		return corechat.Part{}, errors.New("openai: text reasoning is required")
	}
	part := corechat.NewReasoningPart(text, nil)
	if err := part.Metadata.Set(textReasoningProviderKey, provider); err != nil {
		return corechat.Part{}, err
	}
	if err := part.Metadata.Set(textReasoningFieldKey, field); err != nil {
		return corechat.Part{}, err
	}
	return part, nil
}
