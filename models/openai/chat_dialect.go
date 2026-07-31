package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"

	corechat "github.com/Tangerg/lynx/core/chat"
)

const reasoningContentField = "reasoning_content"

// RequestDialect owns provider-specific request semantics layered on top of
// the standard OpenAI Chat Completions wire shape. Source is read-only;
// implementations may mutate only target and must be safe for concurrent use
// when their Chat is shared.
type RequestDialect interface {
	PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error
}

// ResponseDialect owns provider-specific response fields for both complete
// messages and streaming deltas. Source is read-only; implementations may
// mutate only target and must be safe for concurrent use when their Chat is
// shared.
type ResponseDialect interface {
	FinalizeMessage(source openaisdk.ChatCompletionMessage, target *corechat.Message) error
	FinalizeDelta(source openaisdk.ChatCompletionChunkChoiceDelta, target *corechat.Message) error
}

// Dialect groups the independently typed request and response protocol facets
// selected by a provider adapter. One Chat has exactly one dialect.
type Dialect struct {
	Request  RequestDialect
	Response ResponseDialect
}

// ReasoningContentDialect maps the common reasoning_content extension while
// treating it as output-only state.
func ReasoningContentDialect() Dialect {
	codec := reasoningContentCodec{}
	return Dialect{Response: codec}
}

// ReasoningContentToolReplayDialect maps reasoning_content and sends it back
// only on assistant messages containing tool calls.
func ReasoningContentToolReplayDialect() Dialect {
	codec := reasoningContentCodec{replayWithToolCalls: true}
	return Dialect{Request: codec, Response: codec}
}

type reasoningContentCodec struct {
	replayWithToolCalls bool
}

func (codec reasoningContentCodec) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	wireIndex := 0
	for i := range source.Messages {
		message := source.Messages[i]
		if message.Role == corechat.RoleAssistant {
			if wireIndex >= len(target.Messages) || target.Messages[wireIndex].OfAssistant == nil {
				return fmt.Errorf("messages[%d]: assistant wire mapping is missing", i)
			}
			reasoning, hasToolCalls := assistantReplayState(message)
			if reasoning != "" && codec.replayWithToolCalls && hasToolCalls {
				target.Messages[wireIndex].OfAssistant.SetExtraFields(map[string]any{reasoningContentField: reasoning})
			}
		}
		wireIndex += wireMessageCount(message)
	}
	if wireIndex != len(target.Messages) {
		return fmt.Errorf("wire message count = %d; mapped source count = %d", len(target.Messages), wireIndex)
	}
	return nil
}

func (codec reasoningContentCodec) FinalizeMessage(source openaisdk.ChatCompletionMessage, target *corechat.Message) error {
	return prependReasoningContent(source.JSON.ExtraFields, target)
}

func (codec reasoningContentCodec) FinalizeDelta(source openaisdk.ChatCompletionChunkChoiceDelta, target *corechat.Message) error {
	return prependReasoningContent(source.JSON.ExtraFields, target)
}

func assistantReplayState(message corechat.Message) (reasoning string, hasToolCalls bool) {
	var builder strings.Builder
	for i := range message.Parts {
		switch message.Parts[i].Kind {
		case corechat.PartReasoning:
			builder.WriteString(message.Parts[i].Text)
		case corechat.PartToolCall:
			hasToolCalls = true
		}
	}
	return builder.String(), hasToolCalls
}

func wireMessageCount(message corechat.Message) int {
	if message.Role == corechat.RoleTool {
		return len(message.Parts)
	}
	return 1
}

func prependReasoningContent(fields map[string]respjson.Field, target *corechat.Message) error {
	field, ok := fields[reasoningContentField]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return nil
	}
	var reasoning string
	if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err != nil {
		return fmt.Errorf("decode %s: %w", reasoningContentField, err)
	}
	if reasoning == "" {
		return nil
	}
	target.Parts = append([]corechat.Part{corechat.NewReasoningPart(reasoning, nil)}, target.Parts...)
	return nil
}
