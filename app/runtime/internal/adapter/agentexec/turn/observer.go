package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// turnObserver bridges the engine's tool observer to the turn's event
// channel. Each Approve / Start / End notification is translated into a
// ToolCallStart / ToolCallEnd event so transport adapters surface them
// verbatim.
type turnObserver struct {
	dispatcher *memoryDispatcher
	st         *turnState
}

// ApproveToolCall is the non-blocking gate the engine consults BEFORE
// every tool call (HITL R model). It maps the runtime approval mode +
// the tool's safety class to a verdict:
//
//   - auto-pass mode → run the tool.
//   - deny stance (read-only) → recoverable denial, the model adapts.
//   - prompt stance → runtime suspension: the first pass returns a durable
//     Suspension error (the tool loop exits, the action parks at
//     StatusWaiting, the client answers via runs.resume); on resume the gate
//     is consulted again at the same pending call and Interrupt returns the
//     human's [interrupts.Resolution], so the gate runs / denies /
//     runs-with-edited-args accordingly.
//
// The interrupt key is the stable tool name + arguments rather than an
// adapter-generated lifecycle ID. That semantic key survives persisted
// records from older runtimes and still identifies the same gated call when it
// is re-presented on resume. This is the one interrupt mental model shared by
// every HITL flavor.
func (t *turnObserver) ApproveToolCall(ctx context.Context, callID, toolName, arguments string) agentexec.ToolApprovalVerdict {
	// task is pure orchestration. Its child tools are independently observed and
	// gated, while SubagentStart/SubagentStop own the task lifecycle hooks.
	// Running tool hooks or approval for task itself would double-count the
	// orchestration and cannot be replayed faithfully across a child suspension.
	if toolName == "task" {
		return agentexec.ToolApprovalVerdict{}
	}

	// A resumed suspension already contains the durable gate plan built on the
	// first pass. Reuse it before consulting hooks or policy: PreToolUse must run
	// once per logical call, and a restart must preserve its argument rewrite.
	if verdict, handled := t.resumedToolVerdict(ctx, toolName); handled {
		return verdict
	}

	// PreToolUse hooks run first (HITL R model is unaffected): a hook may DENY
	// the call (final), REWRITE its arguments (flows to the gate + the tool), or
	// ASK — escalate a call the gate would pass into a human prompt. A rewrite
	// rides through on the allow paths via verdict.Arguments.
	var hookDecision approval.HookDecision
	if !t.st.hooks.Empty() {
		dec := t.st.hooks.Run(ctx, hooks.Input{
			Event: hooks.PreToolUse, SessionID: t.st.handle.SessionID, Cwd: t.st.cwd,
			Tool: &hooks.ToolInput{Name: toolName, Arguments: arguments},
		})
		hookDecision = approval.HookDecision{
			Block:            dec.Block,
			Reason:           dec.Reason,
			Ask:              dec.Ask,
			RewriteArguments: dec.RewriteArguments,
		}
	}

	mode := approval.ModeYolo
	approvalConfigured := t.dispatcher.approval != nil
	if approvalConfigured {
		var err error
		mode, err = t.dispatcher.approval.Mode(ctx)
		if err != nil {
			return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: "approval mode unavailable"}
		}
	}

	plan := approval.ToolCallInput{
		Tool:               toolName,
		Arguments:          arguments,
		Mode:               mode,
		ApprovalConfigured: approvalConfigured,
		Hook:               hookDecision,
	}.Plan()
	sessionID := t.st.handle.SessionID
	if plan.Action == approval.GatePrompt {
		var d approval.Decision
		var ok bool
		if approvalConfigured {
			query := approval.Query{SessionID: sessionID, ProjectDir: t.st.cwd, Tool: toolName, Arguments: plan.Arguments}
			d, ok, _ = t.dispatcher.approval.Decide(ctx, query)
		}
		autoApproved := false
		// A per-server auto-approve whitelist skips the prompt only after
		// standing rules, so an explicit remembered deny is never overridden.
		if t.dispatcher.mcpToolAutoApproved != nil {
			autoApproved = t.dispatcher.mcpToolAutoApproved(toolName)
		}
		plan = plan.ResolvePromptShortcuts(approval.StandingDecision{Decision: d, Matched: ok}, autoApproved)
	}

	switch plan.Action {
	case approval.GatePass:
		return agentexec.ToolApprovalVerdict{Arguments: plan.ArgumentOverride}
	case approval.GateDeny:
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: plan.DenyReason}
	}

	// interrupt for human approval (R model). First pass bubbles the
	// Suspension error up to park; resume delivers the resolution here. The
	// prompt carries the gated tool's risk so the approval card shows it.
	pending := runs.Interrupt{
		Kind: runs.ApprovalInterruptKind,
		Approval: &runs.ApprovalPrompt{
			CallID: callID, ToolName: toolName, Arguments: plan.Arguments,
			SafetyClass: plan.SafetyClass, Risk: plan.Risk, Reason: plan.PromptReason,
		},
	}
	if err := pending.Validate(); err != nil {
		return agentexec.ToolApprovalVerdict{
			Interrupt: fmt.Errorf("turn: build approval interrupt: %w", err),
		}
	}
	res, err := hitl.Interrupt[interrupts.Resolution](ctx,
		interrupts.InterruptKey(string(runs.ApprovalInterruptKind), toolName, plan.Arguments),
		pending,
	)
	if err != nil {
		return agentexec.ToolApprovalVerdict{Interrupt: err, Arguments: plan.Arguments}
	}
	// "remember{scope}" persists this decision as a rule so matching future
	// calls auto-resolve the same way — recorded for approve AND deny. Keyed on
	// the ORIGINAL arguments (the model regenerates calls like this one); any
	// editedArgs override stays one-shot, never folded into the rule.
	if res.RememberScope != "" && t.dispatcher.approval != nil {
		_ = t.dispatcher.approval.Remember(ctx, approval.RememberRequest{
			Scope:      approval.Scope(res.RememberScope),
			SessionID:  sessionID,
			ProjectDir: t.st.cwd,
			Tool:       toolName,
			Arguments:  plan.Arguments,
			Decision:   approval.DecisionOf(res.Approved),
		})
	}
	if !res.Approved {
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: denialReason(res.Reason)}
	}
	// The human's edited args win over a hook rewrite; fall back to the rewrite
	// when they approved without editing.
	return agentexec.ToolApprovalVerdict{Arguments: plan.ApprovedArguments(res.Arguments)}
}

// resumedToolVerdict recognizes a responded application-owned suspension for
// this tool. Approval responses terminate the gate directly; question
// responses restore the effective arguments and let the question tool consume
// the same response at its hitl.Interrupt call site.
func (t *turnObserver) resumedToolVerdict(ctx context.Context, toolName string) (agentexec.ToolApprovalVerdict, bool) {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return agentexec.ToolApprovalVerdict{}, false
	}
	suspension := process.Suspension()
	if suspension == nil || !suspension.Responded() {
		return agentexec.ToolApprovalVerdict{}, false
	}
	pending, err := runs.DecodeInterrupt(suspension.Prompt)
	if err != nil {
		return agentexec.ToolApprovalVerdict{
			Interrupt: fmt.Errorf("turn: decode responded tool interrupt: %w", err),
		}, true
	}
	pendingTool, effectiveArguments := pending.Tool()
	if pendingTool != toolName {
		return agentexec.ToolApprovalVerdict{}, false
	}

	switch pending.Kind {
	case runs.QuestionInterruptKind:
		return agentexec.ToolApprovalVerdict{Arguments: effectiveArguments}, true
	case runs.ApprovalInterruptKind:
		var resolution interrupts.Resolution
		if err := json.Unmarshal(suspension.Response, &resolution); err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: decode approval resolution: %w", err),
			}, true
		}
		if resolution.RememberScope != "" && t.dispatcher.approval != nil {
			_ = t.dispatcher.approval.Remember(ctx, approval.RememberRequest{
				Scope:      approval.Scope(resolution.RememberScope),
				SessionID:  t.st.handle.SessionID,
				ProjectDir: t.st.cwd,
				Tool:       toolName,
				Arguments:  effectiveArguments,
				Decision:   approval.DecisionOf(resolution.Approved),
			})
		}
		if !resolution.Approved {
			return agentexec.ToolApprovalVerdict{
				Denied: true, DenyReason: denialReason(resolution.Reason),
			}, true
		}
		if resolution.Arguments != "" {
			effectiveArguments = resolution.Arguments
		}
		return agentexec.ToolApprovalVerdict{Arguments: effectiveArguments}, true
	default:
		return agentexec.ToolApprovalVerdict{
			Interrupt: fmt.Errorf("turn: unsupported responded interrupt kind %q", pending.Kind),
		}, true
	}
}

func denialReason(reason string) string {
	if reason == "" {
		return "tool call denied by user"
	}
	return reason
}

func (t *turnObserver) OnToolCallStart(callID, toolName, arguments string) {
	t.dispatcher.emit(t.st, ToolCallStart{
		CallID:      callID,
		ToolName:    toolName,
		Arguments:   arguments,
		SafetyClass: tool.SafetyClassFor(toolName),
	})
}

func (t *turnObserver) OnToolCallEnd(callID, toolName, arguments, output string, mutatedPaths []string, err error) {
	// HITL interrupt: the tool
	// paused for human input. Not a failure — skip the ToolCallEnd
	// event. The turn-park handler drains the in-flight tool item
	// and creates the appropriate interrupt card.
	if hitl.IsInterrupt(err) {
		return
	}
	result := decodeToolResult(output)
	end := ToolCallEnd{
		CallID:       callID,
		Arguments:    arguments,
		Result:       result,
		OutputText:   toolOutputText(toolName, result),
		MutatedPaths: mutatedPaths,
	}
	switch {
	case errors.Is(err, agentexec.ErrToolDenied):
		end.Denied = true // a verdict denial, not an execution failure
	case err != nil:
		end.Err = err.Error()
	}
	t.dispatcher.emit(t.st, end)

	// After a successful todo_write, project the model's (whole-replaced) task
	// list so a client renders the task panel (state.snapshot{todos}); the tool
	// result itself is model-facing only. Read the canonical list from the
	// store rather than the tool args, so the projection can't drift from the
	// arg schema.
	if err == nil && toolName == "todo_write" && t.dispatcher.todos != nil {
		if items, lerr := t.dispatcher.todos.List(t.st.ctx, t.st.handle.SessionID); lerr == nil {
			t.dispatcher.emit(t.st, TodosUpdated{Todos: items})
		}
	}

	// PostToolUse hooks (observe-only in v1): fire after the result so a user
	// script can audit / notify / integrate. Result-injection isn't plumbed yet
	// — the result already streamed to the model — so the Decision is ignored.
	if toolName != "task" && !t.st.hooks.Empty() {
		_ = t.st.hooks.Run(t.st.ctx, hooks.Input{
			Event: hooks.PostToolUse, SessionID: t.st.handle.SessionID, Cwd: t.st.cwd,
			Tool: &hooks.ToolInput{Name: toolName, Result: output}, Reason: errorString(err),
		})
	}
}

func (t *turnObserver) OnMessageDelta(text string) {
	t.dispatcher.emit(t.st, MessageDelta{
		Text: text,
	})
}

// OnReasoningDelta forwards extended-thinking chunks to the turn
// channel as [ReasoningDelta] events. Clients that don't care
// about reasoning can ignore the type in their dispatch switch —
// no event is dropped on the engine side.
func (t *turnObserver) OnReasoningDelta(text string) {
	t.dispatcher.emit(t.st, ReasoningDelta{
		Text: text,
	})
}

// OnUsage forwards the per-round cumulative usage as a [UsageReported] event —
// the mid-run token / cost readout (transport maps it to segment.progress).
// contextTokens is this round's prompt size (the live context occupancy).
func (t *turnObserver) OnUsage(usage accounting.TokenUsage, costUSD float64, contextTokens int64) {
	t.dispatcher.emit(t.st, UsageReported{
		TokenUsage:    usage,
		CostUSD:       costUSD,
		ContextTokens: contextTokens,
	})
}
