package kernel

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	coremodel "github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
)

// inflightTailKey holds, on the process blackboard, the resumable tail a
// HITL interrupt parks. Used only when no ParkStore is configured —
// the nil-ParkStore design keeps the tail on the conversation's blackboard
// instead of a durable store.
const inflightTailKey = "lyra:hitl:inflight-tail"

type resumableInterrupt interface {
	coremodel.Halt
	Awaitable() core.Awaitable
}

// Interrupt delegates to [agent/hitl.Interrupt] so runtime outer layers can
// park on human-in-the-loop awaitables without importing the hitl package
// directly.
func Interrupt[R any](ctx context.Context, key string, value any) (R, bool, error) {
	return hitl.Interrupt[R](ctx, key, value)
}

// InterruptKey is the stable hash input for HITL keys:
// kind/tool/args, with a separator.
//
// This keeps all resumable-key derivation in one place and guarantees the same
// digest shape across approval and question-style interrupts.
func InterruptKey(kind, toolName, arguments string) string {
	return interrupts.InterruptKey(kind, toolName, arguments)
}

// IsInterrupt reports whether err is a resumable HITL-like halt (non-aborting
// and carrying an awaitable payload).
func IsInterrupt(err error) bool {
	h, ok := errors.AsType[resumableInterrupt](err)
	return ok && !h.Abort()
}

// HandleInterrupt parks the process for a resumable hitl-like halt.
//
// This is intentionally generic and stays in runtime: the engine only cares
// about the control-flow contract (non-aborting + awaitable), not the concrete
// hitl package type.
func HandleInterrupt(ctx context.Context, pc *core.ProcessContext, err error) (core.ActionStatus, bool) {
	h, ok := errors.AsType[resumableInterrupt](err)
	if !ok || h.Abort() {
		return 0, false
	}
	return pc.AwaitInput(ctx, h.Awaitable()), true
}

// isInterruptResult reports whether a streamed response is the tool loop's
// interrupt tail rather than model output. Only reached
// when no ParkStore is configured — with a ParkStore the tool
// middleware saves state internally and never yields these.
func isInterruptResult(resp *chat.Response) bool {
	return resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
		resp.Result.Metadata.FinishReason == toolloop.FinishReasonInterrupt
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
