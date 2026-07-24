package turn

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools"
)

// doomLoopThreshold is how many consecutive identical, no-new-output calls must
// complete before the next such call is braked (T13). Read-only tools re-run
// with the same args and same result are pure waste; three in a row is a strong
// loop signal while still tolerating a normal retry or two.
const doomLoopThreshold = 3

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
func (t *turnObserver) ApproveToolCall(ctx context.Context, callID, toolName, arguments string, target agentexec.ToolApprovalTarget) agentexec.ToolApprovalVerdict {
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
		FileMutation:       fileMutationScope(target.FileMutations, cmp.Or(hookDecision.RewriteArguments, arguments), t.st.cwd),
		ShellCommand:       shellCommandFromArguments(toolName, cmp.Or(hookDecision.RewriteArguments, arguments)),
	}.Plan()
	sessionID := t.st.handle.SessionID
	var rememberedArguments tool.Arguments
	if plan.Action == approval.GatePrompt {
		var err error
		rememberedArguments, err = tool.ParseArguments(plan.Arguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: validate gated tool %q arguments: %w", toolName, err),
			}
		}
		var d approval.Decision
		var ok bool
		if approvalConfigured {
			query := approval.Query{SessionID: sessionID, ProjectDir: t.st.cwd, Tool: toolName, Arguments: rememberedArguments}
			d, ok, err = t.dispatcher.approval.Decide(ctx, query)
			if err != nil {
				return agentexec.ToolApprovalVerdict{
					Interrupt: fmt.Errorf("turn: evaluate remembered approval for tool %q: %w", toolName, err),
				}
			}
		}
		autoApproved := false
		// A per-server auto-approve whitelist skips the prompt only after
		// standing rules, so an explicit remembered deny is never overridden.
		if t.dispatcher.mcpToolAutoApproved != nil && target.MCP != (mcpserver.ToolRef{}) {
			autoApproved = t.dispatcher.mcpToolAutoApproved(target.MCP)
		}
		plan = plan.ResolvePromptShortcuts(approval.StandingDecision{Decision: d, Matched: ok}, autoApproved)
	}

	switch plan.Action {
	case approval.GatePass:
		// Doom-loop brake (T13): a call that would auto-pass but has already run
		// identically with no new output enough times is escalated to a human
		// prompt — a would-deny or would-prompt call is untouched (policy already
		// gates it), so this only adds a brake where the model runs unchecked.
		if t.st.repeatedNoProgress(toolName, plan.Arguments) >= doomLoopThreshold {
			return t.doomLoopEscalation(ctx, callID, toolName, plan.Arguments, plan.SafetyClass)
		}
		return agentexec.ToolApprovalVerdict{Arguments: plan.ArgumentOverride}
	case approval.GateDeny:
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: approvalDenialMessage(plan.Denial, toolName)}
	}

	// First pass bubbles the suspension up to park; resume delivers the
	// resolution here. Ordinary policy prompts may create standing rules.
	res, err := t.awaitApproval(ctx, toolName, plan.Arguments, runs.ApprovalPrompt{
		CallID: callID, ToolName: toolName, Arguments: plan.Arguments,
		SafetyClass: plan.SafetyClass, Risk: plan.Risk, Reason: approvalPromptReason(plan.PromptCause),
		Rememberable: true,
	})
	if err != nil {
		return agentexec.ToolApprovalVerdict{Interrupt: err, Arguments: plan.Arguments}
	}
	// "remember{scope}" persists this decision as a rule so matching future
	// calls auto-resolve the same way — recorded for approve AND deny. Keyed on
	// the ORIGINAL arguments (the model regenerates calls like this one); any
	// editedArgs override stays one-shot, never folded into the rule.
	if err := t.rememberApproval(ctx, toolName, rememberedArguments, res); err != nil {
		return agentexec.ToolApprovalVerdict{Interrupt: err}
	}
	// The human's edited args win over a hook rewrite; fall back to the rewrite
	// when they approved without editing.
	return approvalResolutionVerdict(res, plan.ArgumentOverride)
}

func shellCommandFromArguments(toolName, raw string) string {
	if toolName != "shell" {
		return ""
	}
	arguments, err := tool.ParseArguments(raw)
	if err != nil {
		return ""
	}
	command, _ := arguments.StringField("command")
	return command
}

// doomLoopEscalation brakes a model repeating the same call to no effect. It
// raises the ordinary approval interrupt (reusing its resume, UI card, and
// auto-deny-when-unanswerable machinery — a headless client that cannot answer
// approvals auto-denies, braking the loop automatically) with a reason naming
// the loop. The no-progress streak is reset as the brake fires, so after the
// human's decision the model gets a fresh run of calls before it can trip again;
// on denial it also receives recoverable feedback and must change approach. No
// standing rule is consulted or recorded — this is a one-off brake, not a
// persistent permission.
func (t *turnObserver) doomLoopEscalation(ctx context.Context, callID, toolName, arguments string, safetyClass tool.SafetyClass) agentexec.ToolApprovalVerdict {
	t.st.resetDoomLoop()
	res, err := t.awaitApproval(ctx, toolName, arguments, runs.ApprovalPrompt{
		CallID:      callID,
		ToolName:    toolName,
		Arguments:   arguments,
		SafetyClass: safetyClass,
		Risk:        tool.RiskHigh,
		Reason: fmt.Sprintf("%q has been called with the same arguments and no new result %d times in a row — it may be stuck in a loop. Approve to let it continue, or deny to make the agent try a different approach.",
			toolName, doomLoopThreshold),
	})
	if err != nil {
		return agentexec.ToolApprovalVerdict{Interrupt: err, Arguments: arguments}
	}
	return approvalResolutionVerdict(res, arguments)
}

func (t *turnObserver) awaitApproval(ctx context.Context, toolName, arguments string, prompt runs.ApprovalPrompt) (interrupts.Resolution, error) {
	pending := runs.Interrupt{Kind: runs.ApprovalInterruptKind, Approval: &prompt}
	if err := pending.Validate(); err != nil {
		return interrupts.Resolution{}, fmt.Errorf("turn: build approval interrupt: %w", err)
	}
	return suspension.Interrupt(ctx, interrupts.InterruptKey(string(runs.ApprovalInterruptKind), toolName, arguments), pending)
}

func (t *turnObserver) rememberApproval(ctx context.Context, toolName string, arguments tool.Arguments, resolution interrupts.Resolution) error {
	if resolution.RememberScope == "" || t.dispatcher.approval == nil {
		return nil
	}
	if err := t.dispatcher.approval.Remember(ctx, approval.RememberRequest{
		Scope:      resolution.RememberScope,
		SessionID:  t.st.handle.SessionID,
		ProjectDir: t.st.cwd,
		Tool:       toolName,
		Arguments:  arguments,
		Decision:   approval.DecisionOf(resolution.Approved),
	}); err != nil {
		return fmt.Errorf("turn: remember approval decision for tool %q: %w", toolName, err)
	}
	return nil
}

func approvalResolutionVerdict(resolution interrupts.Resolution, fallbackArguments string) agentexec.ToolApprovalVerdict {
	if !resolution.Approved {
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: denialReason(resolution.Reason)}
	}
	return agentexec.ToolApprovalVerdict{Arguments: cmp.Or(resolution.Arguments, fallbackArguments)}
}

func fileMutationScope(reporter tools.FileMutationReporter, arguments, cwd string) tool.FileMutationScope {
	if reporter == nil || cwd == "" {
		return tool.FileMutationNone
	}
	paths, err := reporter.MutationPaths(arguments)
	if err != nil {
		return tool.FileMutationUnknown
	}
	if len(paths) == 0 {
		return tool.FileMutationNone
	}

	root, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return tool.FileMutationUnknown
	}
	for _, path := range paths {
		target, resolveErr := pathidentity.Resolve(root, path)
		if resolveErr != nil {
			return tool.FileMutationUnknown
		}
		inside, compareErr := pathidentity.Contains(root, target)
		if compareErr != nil {
			return tool.FileMutationUnknown
		}
		if !inside {
			return tool.FileMutationOutsideWorkspace
		}
	}
	return tool.FileMutationWithinWorkspace
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
	parked := process.Suspension()
	if parked == nil || !parked.Responded() {
		return agentexec.ToolApprovalVerdict{}, false
	}
	pending, err := suspension.DecodePrompt(parked.Prompt)
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
		rememberedArguments, err := tool.ParseArguments(effectiveArguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: validate restored approval tool %q arguments: %w", toolName, err),
			}, true
		}
		resolution, err := suspension.DecodeResolution(parked.Response)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: decode approval resolution: %w", err),
			}, true
		}
		if pending.Approval.Rememberable {
			if err := t.rememberApproval(ctx, toolName, rememberedArguments, resolution); err != nil {
				return agentexec.ToolApprovalVerdict{Interrupt: err}, true
			}
		}
		return approvalResolutionVerdict(resolution, effectiveArguments), true
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
	t.dispatcher.emit(t.st, runs.ToolCallStart{
		CallID:      callID,
		ToolName:    toolName,
		Arguments:   arguments,
		Activity:    toolActivity(toolName),
		SafetyClass: tool.SafetyClassFor(toolName),
	})
}

func (t *turnObserver) OnToolCallEnd(callID, toolName, arguments, output string, ref *offload.Ref, mutatedPaths []string, err error) {
	// HITL interrupt: the tool
	// paused for human input. Not a failure — skip the ToolCallEnd
	// event. The turn-park handler drains the in-flight tool item
	// and creates the appropriate interrupt card.
	if hitl.IsInterrupt(err) {
		return
	}
	// Feed the doom-loop brake (T13): a completed call — success, error, or a
	// recoverable denial — with the same args and same output as the previous run
	// is a no-progress repeat. The gate reads this count before the next call.
	t.st.recordToolOutcome(toolName, arguments, output)
	result := decodeToolResult(toolName, arguments, output)
	end := runs.ToolCallEnd{
		CallID:       callID,
		Arguments:    arguments,
		Result:       result,
		Offload:      ref,
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
			t.dispatcher.emit(t.st, runs.TodosUpdated{Todos: items})
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
	t.dispatcher.emit(t.st, runs.MessageDelta{
		Text: text,
	})
}

// OnReasoningDelta forwards extended-thinking chunks to the turn
// channel as [ReasoningDelta] events. Clients that don't care
// about reasoning can ignore the type in their dispatch switch —
// no event is dropped on the engine side.
func (t *turnObserver) OnReasoningDelta(text string) {
	t.dispatcher.emit(t.st, runs.ReasoningDelta{
		Text: text,
	})
}

// OnUsage forwards the per-round cumulative usage as a [UsageReported] event —
// the mid-run token / cost readout (transport maps it to segment.progress).
// contextTokens is this round's prompt size (the live context occupancy).
func (t *turnObserver) OnUsage(usage accounting.TokenUsage, costUSD float64, contextTokens int64) {
	t.dispatcher.emit(t.st, runs.UsageReported{
		TokenUsage:    usage,
		CostUSD:       costUSD,
		ContextTokens: contextTokens,
	})
}
