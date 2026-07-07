package kernel

import (
	"encoding/json"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// inflightTailKey holds, on the process blackboard, the resumable tail a
// HITL interrupt parks. Used only when no ParkStore is configured —
// the nil-ParkStore design keeps the tail on the conversation's blackboard
// instead of a durable store.
const inflightTailKey = "lyra:hitl:inflight-tail"
const finishReasonInterrupt = "interrupt"

// isInterruptResult reports whether a streamed response is the tool loop's
// interrupt tail rather than model output. Only reached
// when no ParkStore is configured — with a ParkStore the tool
// middleware saves state internally and never yields these.
func isInterruptResult(resp *chat.Response) bool {
	return resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
		resp.Result.Metadata.FinishReason == chat.FinishReason(finishReasonInterrupt)
}

type inflightTailStore struct {
	bb core.Blackboard
}

func (s inflightTailStore) Save(result *chat.Result) {
	if result == nil || result.AssistantMessage == nil {
		return
	}
	tail := []chat.Message{result.AssistantMessage}
	if result.ToolMessage != nil {
		tail = append(tail, result.ToolMessage)
	}
	data, err := marshalMessages(tail)
	if err != nil {
		return
	}
	s.bb.Set(inflightTailKey, data)
}

func (s inflightTailStore) Load() ([]chat.Message, bool) {
	v, ok := s.bb.Get(inflightTailKey)
	if !ok {
		return nil, false
	}
	data, ok := v.(string)
	if !ok || data == "" {
		return nil, false
	}
	msgs, err := unmarshalMessages(data)
	if err != nil || len(msgs) == 0 {
		return nil, false
	}
	return msgs, true
}

func (s inflightTailStore) Clear() {
	s.bb.Set(inflightTailKey, "")
}

func marshalMessages(msgs []chat.Message) (string, error) {
	raws := make([]json.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return "", err
		}
		raws = append(raws, b)
	}
	b, err := json.Marshal(raws)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalMessages(data string) ([]chat.Message, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(data), &raws); err != nil {
		return nil, err
	}
	msgs := make([]chat.Message, 0, len(raws))
	for _, raw := range raws {
		m, err := chat.UnmarshalMessage(raw)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}
