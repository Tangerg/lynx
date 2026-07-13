package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/toolloop"
	coremodel "github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
)

// inflightTailKey holds the resumable tail of a HITL interrupt on the process
// blackboard. The process snapshot is the runtime's single durable source of
// truth, so the tail is restored atomically with the rest of the process.
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
// interrupt tail rather than model output.
func isInterruptResult(resp *chat.Response) bool {
	return resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
		resp.Result.Metadata.FinishReason == toolloop.FinishReasonInterrupt
}

type inflightTailStore struct {
	bb core.Blackboard
}

// ValidateInterruptSnapshot verifies that a waiting process snapshot contains
// the exact tool-loop tail runTurn needs to resume. The composition root passes
// this capability to boot reconciliation so storage can validate executor-
// specific state without importing this adapter.
func ValidateInterruptSnapshot(snapshot core.ProcessSnapshot) error {
	tag, ok := snapshot.Blackboard[inflightTailKey]
	if !ok || len(tag.Value) == 0 {
		return errors.New("agentexec: interrupt snapshot has no resumable tail")
	}
	var data string
	if err := json.Unmarshal(tag.Value, &data); err != nil {
		return fmt.Errorf("agentexec: decode snapshot interrupt-tail binding: %w", err)
	}
	msgs, err := unmarshalMessages(data)
	if err != nil {
		return fmt.Errorf("agentexec: decode snapshot interrupt tail: %w", err)
	}
	if len(msgs) < 1 || len(msgs) > 2 {
		return fmt.Errorf("agentexec: snapshot interrupt tail has %d messages, want 1 or 2", len(msgs))
	}
	assistant, ok := msgs[0].(*chat.AssistantMessage)
	if !ok || !assistant.HasToolCalls() {
		return errors.New("agentexec: snapshot interrupt tail does not start with assistant tool calls")
	}
	if len(msgs) == 2 {
		toolMessage, ok := msgs[1].(*chat.ToolMessage)
		if !ok || len(toolMessage.ToolReturns) == 0 {
			return errors.New("agentexec: snapshot interrupt tail has an invalid completed-tool message")
		}
	}
	return nil
}

func (s inflightTailStore) Save(result *chat.Result) error {
	if result == nil || result.AssistantMessage == nil {
		return errors.New("agentexec: interrupt tail has no assistant message")
	}
	tail := []chat.Message{result.AssistantMessage}
	if result.ToolMessage != nil {
		tail = append(tail, result.ToolMessage)
	}
	data, err := marshalMessages(tail)
	if err != nil {
		return fmt.Errorf("agentexec: encode interrupt tail: %w", err)
	}
	s.bb.Set(inflightTailKey, data)
	return nil
}

func (s inflightTailStore) Load() ([]chat.Message, bool, error) {
	v, ok := s.bb.Get(inflightTailKey)
	if !ok {
		return nil, false, nil
	}
	data, ok := v.(string)
	if !ok {
		return nil, false, fmt.Errorf("agentexec: interrupt tail has type %T, want string", v)
	}
	if data == "" {
		return nil, false, nil
	}
	msgs, err := unmarshalMessages(data)
	if err != nil {
		return nil, false, fmt.Errorf("agentexec: decode interrupt tail: %w", err)
	}
	if len(msgs) == 0 {
		return nil, false, errors.New("agentexec: interrupt tail is empty")
	}
	return msgs, true, nil
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
