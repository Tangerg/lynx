package chat

// Thinking metadata keys mirror Spring AI's approach: instead of adding a
// dedicated field to AssistantMessage for reasoning/thinking content, we
// rely on Response.Results being a list and AssistantMessage.Metadata
// being an open map.
//
// Two patterns coexist (also chosen per provider in Spring AI):
//
//  1. Multi-Result pattern (Anthropic-style): a single Response carries one
//     Result per ContentBlock. Thinking blocks become standalone Results
//     whose AssistantMessage.Metadata is tagged with MetaIsThought=true and
//     MetaThinkingSignature (or MetaRedactedThinkingData for safety-redacted
//     blocks).
//
//  2. Metadata-channel pattern (DeepSeek / OpenAI-compatible): a single
//     Result whose AssistantMessage.Metadata carries MetaReasoningContent
//     alongside the regular Text.
//
// Streaming aggregation produces a single AssistantMessage but stores
// MetaThoughts (combined thinking text) and MetaOutputWithoutThoughts
// (combined non-thinking text) so callers can render either view without
// re-scanning chunks.
const (
	// MetaIsThought marks an AssistantMessage as carrying thinking/reasoning
	// content rather than the final assistant response. Value type: bool.
	MetaIsThought = "lynx:chat:is_thought"

	// MetaThinkingSignature carries the cryptographic signature attached to
	// a thinking block (Anthropic, Google Gemini). Required for multi-turn
	// replay when the next request must echo prior thinking blocks back to
	// the API. Value type: string.
	MetaThinkingSignature = "lynx:chat:thinking_signature"

	// MetaRedactedThinkingData carries the opaque safety-redacted payload of
	// an Anthropic redacted_thinking block. The provider returns this in
	// place of visible reasoning text and it must be passed back unchanged
	// in subsequent requests. Value type: string.
	MetaRedactedThinkingData = "lynx:chat:redacted_thinking_data"

	// MetaReasoningContent carries reasoning text returned alongside a
	// regular assistant message via a non-structured field (DeepSeek
	// reasoning_content, OpenAI-compatible servers exposing reasoning text).
	// Used by the metadata-channel pattern. Value type: string.
	MetaReasoningContent = "lynx:chat:reasoning_content"

	// MetaThoughts is set on the aggregated AssistantMessage produced by
	// ResponseAccumulator after a streaming run. It contains the
	// concatenation of all thinking text seen during the stream, regardless
	// of whether the upstream pattern was multi-Result or metadata-channel.
	// Value type: string.
	MetaThoughts = "lynx:chat:thoughts"

	// MetaOutputWithoutThoughts is the streaming-aggregation companion to
	// MetaThoughts: the concatenation of non-thinking text only. Callers
	// rendering UI typically display this view; the standard Text field
	// still contains the full mixed concatenation for backward-compat.
	// Value type: string.
	MetaOutputWithoutThoughts = "lynx:chat:output_without_thoughts"
)

// IsThoughtMessage reports whether the given assistant message represents a
// thinking/reasoning block (multi-Result pattern). Returns false for nil.
func IsThoughtMessage(m *AssistantMessage) bool {
	if m == nil {
		return false
	}
	v, ok := m.Metadata[MetaIsThought].(bool)
	return ok && v
}

// ThinkingSignature returns the signature attached to a thinking block, if
// any. Empty string indicates either a non-thinking message or a thinking
// block whose signature has not yet arrived (e.g., streaming mid-flight).
func ThinkingSignature(m *AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaThinkingSignature].(string)
	return v
}

// RedactedThinkingData returns the opaque payload of a redacted_thinking
// block. Empty string means the message is not a redacted thinking block.
func RedactedThinkingData(m *AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaRedactedThinkingData].(string)
	return v
}

// ReasoningContent returns the inline reasoning text carried via the
// metadata-channel pattern (DeepSeek/OpenAI-compatible). Empty string
// means no reasoning text was provided.
func ReasoningContent(m *AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaReasoningContent].(string)
	return v
}
