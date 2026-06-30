package turn

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"

	corechat "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// firstNonEmptyStr returns the first non-empty argument, or "".
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// turnObserver bridges the engine's tool observer to the turn's event
// channel. Each Approve / Start / End notification is translated into a
// ToolCallStart / ToolCallEnd event so transport adapters surface them
// verbatim.
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
	// SafetyClass / Risk / Reason describe the gated call so the approval card
	// shows the risk + a one-line why without joining tools.list. SafetyClass
	// is the wire class ("write"|"exec"); Risk is the coarse low/medium/high.
	SafetyClass string `json:"safetyClass"`
	Risk        string `json:"risk"`
	Reason      string `json:"reason"`
}

// ApproveToolCall is the non-blocking gate the engine consults BEFORE
// every tool call (HITL R model). It maps the runtime approval mode +
// the tool's safety class to a verdict:
//
//   - auto-pass mode → run the tool.
//   - deny stance (read-only) → recoverable denial, the model adapts.
//   - prompt stance → [hitl.Interrupt]: the first pass returns an
//     InterruptError (the tool loop exits, the action parks at
//     StatusWaiting, the client answers via runs.resume); on resume the gate
//     is consulted again at the same pending call and Interrupt returns the
//     human's [interrupts.Resolution], so the gate runs / denies /
//     runs-with-edited-args accordingly.
//
// The interrupt key is the stable tool name + arguments (NOT the
// per-invocation callID, which is generated fresh on every Call) so the
// recorded resolution matches the same call site when the parked tool call is
// re-presented on resume. This is the one interrupt mental model shared by
// every HITL flavor.
func (t *turnObserver) ApproveToolCall(ctx context.Context, callID, toolName, arguments string) kernel.ToolApprovalVerdict {
	// PreToolUse hooks run first (HITL R model is unaffected): a hook may DENY
	// the call (final), REWRITE its arguments (flows to the gate + the tool), or
	// ASK — escalate a call the gate would pass into a human prompt. A rewrite
	// rides through on the allow paths via verdict.Arguments.
	var rewritten string
	forcePrompt := false
	if !t.st.hooks.Empty() {
		dec := t.st.hooks.Run(ctx, hooks.Input{
			Event: hooks.PreToolUse, SessionID: t.st.handle.SessionID, Cwd: t.st.cwd,
			Tool: &hooks.ToolInput{Name: toolName, Arguments: arguments},
		})
		if dec.Block {
			return kernel.ToolApprovalVerdict{Denied: true, DenyReason: firstNonEmptyStr(dec.Reason, "denied by a PreToolUse hook")}
		}
		if dec.RewriteArguments != "" {
			rewritten = dec.RewriteArguments
			arguments = rewritten
		}
		forcePrompt = dec.Ask
	}

	if t.svc.approval == nil {
		return kernel.ToolApprovalVerdict{Arguments: rewritten} // run (rewritten "" → no override)
	}
	mode, err := t.svc.approval.GetMode(ctx)
	if err != nil {
		return kernel.ToolApprovalVerdict{Denied: true, DenyReason: "approval mode unavailable"}
	}
	cls := tool.SafetyClassFor(toolName)
	action := approval.GateFor(cls, mode)
	if forcePrompt && action == approval.GatePass {
		action = approval.GatePrompt // a PreToolUse hook escalated this call to human review
	}
	switch action {
	case approval.GatePass:
		return kernel.ToolApprovalVerdict{Arguments: rewritten}
	case approval.GateDeny:
		// GateDeny only fires in the read-only plan stance (ModePlan); guide the
		// model back onto the plan-then-exit path rather than just refusing.
		return kernel.ToolApprovalVerdict{Denied: true, DenyReason: "plan mode is active (read-only): " + toolName + " is not permitted. Investigate with read-only tools, then call exit_plan_mode to present your plan for approval."}
	}

	// GatePrompt. A matching standing rule short-circuits the prompt; else
	// interrupt for human approval and persist a rule if "remember".
	sessionID := t.st.handle.SessionID
	query := approval.Query{SessionID: sessionID, ProjectDir: t.st.cwd, Tool: toolName, Arguments: arguments}
	if d, ok, _ := t.svc.approval.Decide(ctx, query); ok {
		if d == approval.Deny {
			return kernel.ToolApprovalVerdict{Denied: true, DenyReason: "tool call denied by a remembered rule"}
		}
		return kernel.ToolApprovalVerdict{Arguments: rewritten} // remembered allow
	}

	// No standing rule. A per-server auto-approve whitelist entry skips the
	// prompt for this MCP tool — a coarser "trust this server's tool" than a
	// remembered rule, so it is consulted AFTER rules (an explicit remembered
	// deny above still wins) and only on this GatePrompt path (it never reaches
	// the read-only plan-mode GateDeny). The set is keyed on the model-facing
	// "<server>_<tool>" the runtime derives from the MCP registry.
	if t.svc.mcpAutoApprove != nil {
		if _, ok := t.svc.mcpAutoApprove()[toolName]; ok {
			return kernel.ToolApprovalVerdict{Arguments: rewritten}
		}
	}

	// interrupt for human approval (R model). First pass bubbles the
	// InterruptError up to park; resume delivers the resolution here. The
	// prompt carries the gated tool's risk so the approval card shows it.
	risk, reason := approval.RiskFor(cls)
	res, _, err := hitl.Interrupt[interrupts.Resolution](ctx,
		approvalKey(toolName, arguments),
		ApprovalPrompt{
			CallID: callID, ToolName: toolName, Arguments: arguments,
			SafetyClass: tool.ClassName(cls), Risk: risk, Reason: reason,
		},
	)
	if err != nil {
		return kernel.ToolApprovalVerdict{Interrupt: err}
	}
	// "remember{scope}" persists this decision as a rule so matching future
	// calls auto-resolve the same way — recorded for approve AND deny. Keyed on
	// the ORIGINAL arguments (the model regenerates calls like this one); any
	// editedArgs override stays one-shot, never folded into the rule.
	if res.RememberScope != "" {
		_ = t.svc.approval.Remember(ctx, approval.RememberRequest{
			Scope:      approval.Scope(res.RememberScope),
			SessionID:  sessionID,
			ProjectDir: t.st.cwd,
			Tool:       toolName,
			Arguments:  arguments,
			Decision:   decisionOf(res.Approved),
		})
	}
	if !res.Approved {
		return kernel.ToolApprovalVerdict{Denied: true, DenyReason: "tool call denied by user"}
	}
	// The human's edited args win over a hook rewrite; fall back to the rewrite
	// when they approved without editing.
	return kernel.ToolApprovalVerdict{Arguments: firstNonEmptyStr(res.Arguments, rewritten)}
}

// decisionOf maps an approve/deny boolean to the approval domain's verdict.
func decisionOf(approved bool) approval.Decision {
	if approved {
		return approval.Allow
	}
	return approval.Deny
}

// approvalKey is the interrupt key for one gated tool call. Keyed by tool
// name + arguments (NOT the per-invocation callID, which is fresh on every
// Call): resume feeds the same parked tool call back unchanged, so keying on
// its name + arguments matches the recorded resolution.
func approvalKey(toolName, arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return "approval." + strconv.FormatUint(h.Sum64(), 16)
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.svc.emit(t.st, ToolCallStart{
		CallID:      callID,
		ToolName:    toolName,
		Arguments:   arguments,
		SafetyClass: tool.ClassName(tool.SafetyClassFor(toolName)),
	})
}

func (t *turnObserver) OnToolCallEnd(callID, toolName, output string, err error) {
	// HITL interrupt (chat.ToolHalt with Abort()==false): the tool
	// paused for human input. Not a failure — skip the ToolCallEnd
	// event. The turn-park handler drains the in-flight tool item
	// and creates the appropriate interrupt card.
	if h, ok := errors.AsType[corechat.ToolHalt](err); ok && !h.Abort() {
		return
	}
	end := ToolCallEnd{CallID: callID, Output: output}
	switch {
	case errors.Is(err, kernel.ErrToolDenied):
		end.Denied = true // a verdict denial, not an execution failure
	case err != nil:
		end.Err = err.Error()
	}
	t.svc.emit(t.st, end)

	// After a successful todo_write, project the model's (whole-replaced) task
	// list so a client renders the task panel (state.snapshot{todos}); the tool
	// result itself is model-facing only. Read the canonical list from the
	// store rather than the tool args, so the projection can't drift from the
	// arg schema.
	if err == nil && toolName == "todo_write" && t.svc.todos != nil {
		if items, lerr := t.svc.todos.List(t.st.ctx, t.st.handle.SessionID); lerr == nil {
			t.svc.emit(t.st, TodosUpdated{Todos: items})
		}
	}

	// PostToolUse hooks (observe-only in v1): fire after the result so a user
	// script can audit / notify / integrate. Result-injection isn't plumbed yet
	// — the result already streamed to the model — so the Decision is ignored.
	if !t.st.hooks.Empty() {
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		_ = t.st.hooks.Run(t.st.ctx, hooks.Input{
			Event: hooks.PostToolUse, SessionID: t.st.handle.SessionID, Cwd: t.st.cwd,
			Tool: &hooks.ToolInput{Name: toolName, Result: output}, Reason: errStr,
		})
	}
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

// OnUsage forwards the per-round cumulative usage as a [UsageReported] event —
// the mid-run token / cost readout (transport maps it to run.progress).
// contextTokens is this round's prompt size (the live context occupancy).
func (t *turnObserver) OnUsage(usage kernel.TokenUsage, costUSD float64, contextTokens int64) {
	t.svc.emit(t.st, UsageReported{
		TokenUsage:    usage,
		CostUSD:       costUSD,
		ContextTokens: contextTokens,
	})
}
