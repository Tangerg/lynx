package chat

import (
	"encoding/json"
	"strings"
)

func (u *UserMessage) Transcript() string {
	return transcript(MessageTypeUser, u.Text)
}

func (s *SystemMessage) Transcript() string {
	return transcript(MessageTypeSystem, s.Text)
}

func (a *AssistantMessage) Transcript() string {
	var b strings.Builder
	b.WriteString(MessageTypeAssistant.String())
	b.WriteString(": ")
	b.WriteString(a.JoinedText())
	if a.HasToolCalls() {
		b.WriteByte('\n')
		calls := a.CollectToolCalls()
		data, _ := json.Marshal(calls)
		b.Write(data)
	}
	return b.String()
}

func (t *ToolMessage) Transcript() string {
	returns, _ := json.Marshal(t.ToolReturns)
	return transcript(MessageTypeTool, string(returns))
}

func transcript(messageType MessageType, payload string) string {
	return messageType.String() + ": " + payload
}

func (l MessageList) Strings() []string {
	out := make([]string, 0, len(l))
	for _, msg := range l {
		out = append(out, msg.Transcript())
	}
	return out
}
