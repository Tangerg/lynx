package anthropic

import "github.com/Tangerg/lynx/core/model/chat"

// MetaRedactedReasoning carries the opaque payload of a
// RedactedThinkingBlock — safety-redacted reasoning that has no
// visible text but must be replayed unchanged on the next turn.
// Stored on [chat.AssistantMessage.Metadata] because there is no
// counterpart [chat.OutputPart] for an opaque blob; visible
// thinking lives on [chat.ReasoningPart] (with its own Signature
// field) and does not need this side channel.
const MetaRedactedReasoning = "lynx:chat:anthropic:redacted_reasoning"

func redactedReasoning(m *chat.AssistantMessage) string {
	if m == nil {
		return ""
	}
	v, _ := m.Metadata[MetaRedactedReasoning].(string)
	return v
}
