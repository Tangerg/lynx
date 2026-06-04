package chat

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// turnObserver bridges engine.ToolObserver to the turn's event
// channel. The engine fires Approve / Start / End for every tool
// the model invokes; we translate each into a Lyra ToolCall*
// event so transport adapters surface them verbatim.
type turnObserver struct {
	svc *inMemory
	st  *turnState
}

// ApprovalPrompt is the awaitable payload surfaced to the client when a
// gated tool call needs approval (HITL R model). It rides the run's
// interrupt outcome; the client answers via a continuation run.
type ApprovalPrompt struct {
	CallID    string `json:"callId"`
	ToolName  string `json:"toolName"`
	Arguments string `json:"arguments"`
}

// ApproveToolCall is the non-blocking gate the engine consults BEFORE
// every tool call (HITL R model). It maps the runtime approval mode +
// the tool's safety class to a verdict:
//
//   - auto-pass mode → run the tool.
//   - deny stance (read-only) → recoverable denial, the model adapts.
//   - prompt stance → consult the per-call ledger on the process
//     blackboard (keyed by the stable tool name + arguments so the
//     verdict survives the LLM round re-running on resume). Undecided →
//     park on a confirmation awaitable; the run suspends at
//     StatusWaiting and the client answers via runs.resume.
//
// Unlike the old blocking gate, this never waits on a channel — the
// decision is delivered out of band by [runtime.Platform.ResumeProcess],
// which runs the awaitable's handler to record the verdict before the
// action re-runs.
func (t *turnObserver) ApproveToolCall(ctx context.Context, callID, toolName, arguments string) engine.ToolApprovalVerdict {
	if t.svc.approval == nil {
		return engine.ToolApprovalVerdict{} // run
	}
	mode, err := t.svc.approval.GetMode(ctx)
	if err != nil {
		return engine.ToolApprovalVerdict{Denied: true, DenyReason: "approval mode unavailable: " + err.Error()}
	}
	switch gateFor(toolName, mode) {
	case gatePass:
		return engine.ToolApprovalVerdict{}
	case gateDeny:
		return engine.ToolApprovalVerdict{Denied: true, DenyReason: "read-only mode: " + toolName + " is not permitted"}
	}

	// gatePrompt: read the per-call ledger off the process blackboard.
	proc := core.ProcessFrom(ctx)
	if proc == nil {
		// No process on ctx (defensive — the agent runtime always wires
		// one). Fail closed rather than run an ungated call.
		return engine.ToolApprovalVerdict{Denied: true, DenyReason: "approval unavailable: no process context"}
	}
	bb := proc.Blackboard()
	key := approvalKey(toolName, arguments)
	if approved, decided := bb.Condition(key); decided {
		if approved {
			return engine.ToolApprovalVerdict{}
		}
		return engine.ToolApprovalVerdict{Denied: true, DenyReason: "tool call denied by user"}
	}

	// Undecided → park on a confirmation. The handler records the verdict
	// on the blackboard at ResumeProcess time, so the re-run observes it.
	prompt := ApprovalPrompt{CallID: callID, ToolName: toolName, Arguments: arguments}
	awaitable := hitl.NewConfirmation(prompt, func(approved bool) core.ResponseImpact {
		bb.SetCondition(key, approved)
		return core.ImpactUpdated
	})
	return engine.ToolApprovalVerdict{Pause: awaitable}
}

// approvalKey is the blackboard condition key for one gated tool call.
// Keyed by tool name + arguments (NOT the per-invocation callID, which
// is regenerated each LLM round): the same frozen context produces the
// same tool call on resume, so the recorded verdict matches.
func approvalKey(toolName, arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return "approval." + strconv.FormatUint(h.Sum64(), 16)
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.svc.emit(t.st, ToolCallStart{
		CallID:    callID,
		ToolName:  toolName,
		Arguments: arguments,
	})
}

func (t *turnObserver) OnToolCallEnd(callID, _ string, output string, err error) {
	end := ToolCallEnd{CallID: callID, Output: output}
	switch {
	case errors.Is(err, engine.ErrToolDenied):
		end.Denied = true // a verdict denial, not an execution failure
	case err != nil:
		end.Err = err.Error()
	}
	t.svc.emit(t.st, end)
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

// OnPlanGenerated forwards the plan the agent drafted (plan mode) as a
// [PlanGenerated] event, just before the process parks on approval. The
// client renders it and replies via [Service.Resume].
func (t *turnObserver) OnPlanGenerated(plan string) {
	t.svc.emit(t.st, PlanGenerated{
		Plan: plan,
	})
}
