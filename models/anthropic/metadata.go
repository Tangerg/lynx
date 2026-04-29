package anthropic

import "github.com/Tangerg/lynx/core/model/chat"

// Metadata keys for Anthropic-specific reasoning continuation tokens.
// Stored on AssistantMessage.Metadata by the response side and read by
// the request side for multi-turn replay. The key string values follow
// the framework-wide "lynx:chat:<provider>:<concept>" namespace
// convention so they cannot collide across providers when serialized.
//
// Business code generally should NOT read these keys directly: pull
// reasoning text from AssistantMessage.Reasoning and let this package
// handle replay internally. Constants are exported only so other
// integrations within the Lynx ecosystem can interop if needed.
const (
	// MetaReasoningSignature is the per-block continuity token Anthropic
	// returns alongside a ThinkingBlock (Anthropic SDK still calls it a
	// "thinking" block; Lynx normalizes the abstraction layer to
	// "reasoning"). Must be replayed verbatim or the API rejects the
	// follow-up request. Value type: string.
	MetaReasoningSignature = "lynx:chat:anthropic:reasoning_signature"

	// MetaRedactedReasoning is the opaque payload of a
	// RedactedThinkingBlock — safety-redacted reasoning that contains
	// no visible text but must be replayed unchanged on the next turn.
	// When present, AssistantMessage.Reasoning will be empty. Value
	// type: string.
	MetaRedactedReasoning = "lynx:chat:anthropic:redacted_reasoning"
)

// reasoningSignature is an internal accessor used by request/response
// helpers in this package. Returns "" when the message lacks a signature
// — the request side treats this as "no replay needed" (the message
// either had no reasoning or arrived from a source that did not capture
// the signature).
func reasoningSignature(m *chat.AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaReasoningSignature].(string)
	return v
}

// redactedReasoning returns the opaque safety-redacted payload, if the
// message represents a redacted thinking block. Empty string otherwise.
func redactedReasoning(m *chat.AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaRedactedReasoning].(string)
	return v
}
