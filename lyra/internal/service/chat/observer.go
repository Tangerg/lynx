package chat

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// turnObserver bridges engine.ToolObserver to the turn's event
// channel. The engine fires Approve / Start / End for every tool
// the model invokes; we translate each into a Lyra ToolCall*
// event so transport adapters surface them verbatim.
type turnObserver struct {
	svc *inMemory
	st  *turnState
}

// OnToolCallApprove is the gate the engine fires BEFORE every tool
// call. When the configured [approval.Service] mode + the tool's
// safety class agree to auto-pass the call, the gate returns nil
// immediately and the tool runs. Otherwise the gate registers the
// pending request, emits a [ToolCallApproval] event onto the turn
// channel, and blocks on the decision channel until the client
// posts a verdict via [approval.Service.Decide].
//
// Returns nil to proceed, an error to short-circuit. The engine
// surfaces the error back to the model as the tool's "output"
// (engine.observedTool collapses Deny into a non-fatal tool
// result) so the model can recover without aborting the turn.
func (t *turnObserver) OnToolCallApprove(ctx context.Context, callID, toolName, arguments string) error {
	if t.svc.approval == nil {
		return nil
	}
	mode, err := t.svc.approval.GetMode(ctx)
	if err != nil {
		return err
	}
	switch gateFor(toolName, mode) {
	case gatePass:
		return nil
	case gateDeny:
		// ModeReadOnly (or any future deny stance): refuse without
		// prompting. The engine surfaces this back to the model as a
		// tool error so it adapts instead of aborting the turn.
		return errors.New("read-only mode: " + toolName + " is not permitted")
	}
	// gatePrompt: register, emit the event, wait for the verdict.

	req := approval.Request{
		ID:          callID,
		SessionID:   t.st.handle.SessionID,
		TurnID:      t.st.handle.TurnID,
		ToolName:    toolName,
		Arguments:   arguments,
		RequestedAt: time.Now(),
	}
	// Register BEFORE emit so a Decide that arrives the instant
	// the client sees the event has a pending entry to resolve.
	decisionCh, cleanup := t.svc.approval.Register(req)
	defer cleanup()

	t.svc.emit(t.st, ToolCallApproval{Request: req})

	select {
	case d := <-decisionCh:
		if d == approval.DecisionDeny {
			return errors.New("tool call denied by user")
		}
		return nil // DecisionApprove
	case <-ctx.Done():
		// Fail-closed on turn cancellation.
		return ctx.Err()
	}
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.svc.emit(t.st, ToolCallStart{
		CallID:    callID,
		ToolName:  toolName,
		Arguments: arguments,
	})
}

func (t *turnObserver) OnToolCallEnd(callID, _ string, output string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	t.svc.emit(t.st, ToolCallEnd{
		CallID: callID,
		Output: output,
		Err:    errStr,
	})
}

func (t *turnObserver) OnMessageDelta(text string) {
	t.svc.emit(t.st, MessageDelta{
		Text: text,
	})
}

// OnReasoningDelta forwards extended-thinking chunks to the turn
// channel as [ReasoningDelta] events. Clients that don't care
// about reasoning can ignore the type in their dispatch switch —
// no event is dropped on the engine side.
func (t *turnObserver) OnReasoningDelta(text string) {
	t.svc.emit(t.st, ReasoningDelta{
		Text: text,
	})
}
