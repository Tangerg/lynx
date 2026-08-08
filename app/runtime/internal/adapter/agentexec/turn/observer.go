package turn

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// doomLoopThreshold is how many consecutive identical, no-new-output calls must
// complete before the next such call is braked (T13). Read-only tools re-run
// with the same args and same result are pure waste; three in a row is a strong
// loop signal while still tolerating a normal retry or two.
const doomLoopThreshold = 3

// turnObserver projects the engine's post-gate tool lifecycle and streaming
// notifications onto the turn's event channel. Approval policy lives in
// toolGate so event projection cannot accidentally own a pre-execution rule.
type turnObserver struct {
	controller       *controller
	st               *turnState
	projectChildRuns bool
	childRunsMu      sync.RWMutex
	childRuns        map[string]runs.ChildRunBinding
	childProcesses   map[string]string
}

func (t *turnObserver) projects(process agentexec.ProcessRef) bool {
	return !process.Child() || (t.projectChildRuns && process.AgentToolChild())
}

// admitChild sends the child opening request through the same serialized
// executor stream as the parent ToolCallStarted that causally precedes it. The
// Agent Runtime blocks child publication and execution until the Coordinator
// confirms the durable transaction.
func (t *turnObserver) admitChild(ctx context.Context, child agentexec.ChildProcess) error {
	request, confirmation := runs.NewChildOpeningRequest(child.StartedAt)
	if !t.controller.emitProcessEvent(t.st, child.ProcessRef, request) {
		switch {
		case ctx != nil && ctx.Err() != nil:
			return fmt.Errorf("turn: request child process %q opening: %w", child.ID, ctx.Err())
		case t.st.ctx.Err() != nil:
			return fmt.Errorf("turn: request child process %q opening: %w", child.ID, t.st.ctx.Err())
		default:
			return fmt.Errorf("turn: request child process %q opening: executor event stream is closed", child.ID)
		}
	}
	binding, err := confirmation.Await(ctx)
	if err != nil {
		return fmt.Errorf("turn: admit child process %q: %w", child.ID, err)
	}
	if binding.ProcessID != child.ID {
		return fmt.Errorf(
			"turn: admitted child process %q returned binding for process %q",
			child.ID,
			binding.ProcessID,
		)
	}
	return t.bindChildRun(binding)
}

func (t *turnObserver) restoreChildRuns(bindings []runs.ChildRunBinding) error {
	for _, binding := range bindings {
		if err := t.bindChildRun(binding); err != nil {
			return fmt.Errorf("turn: restore child Run bindings: %w", err)
		}
	}
	return nil
}

func (t *turnObserver) bindChildRun(binding runs.ChildRunBinding) error {
	if err := binding.Validate(); err != nil {
		return fmt.Errorf("turn: bind child Run: %w", err)
	}
	t.childRunsMu.Lock()
	defer t.childRunsMu.Unlock()
	if t.childRuns == nil {
		t.childRuns = make(map[string]runs.ChildRunBinding)
		t.childProcesses = make(map[string]string)
	}
	if existing, ok := t.childRuns[binding.ProcessID]; ok {
		if existing != binding {
			return fmt.Errorf(
				"turn: child process %q binding changed from Run %q to %q",
				binding.ProcessID,
				existing.RunID,
				binding.RunID,
			)
		}
		return nil
	}
	if processID, ok := t.childProcesses[binding.RunID]; ok {
		return fmt.Errorf(
			"turn: child Run %q is already bound to process %q",
			binding.RunID,
			processID,
		)
	}
	t.childRuns[binding.ProcessID] = binding
	t.childProcesses[binding.RunID] = binding.ProcessID
	return nil
}

func (t *turnObserver) childRun(processID string) (runs.ChildRunBinding, bool) {
	t.childRunsMu.RLock()
	defer t.childRunsMu.RUnlock()
	binding, ok := t.childRuns[processID]
	return binding, ok
}

// toolGate owns the pre-execution protocol for one tool call: hook input,
// standing policy, remembered decisions, HITL suspension, and the doom-loop
// brake. It deliberately does not project tool events or post-tool hooks.
type toolGate struct {
	controller *controller
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
//     StatusWaiting, then a continuation supplies the answer); on resume the gate
//     is consulted again at the same pending call and Interrupt returns the
//     human's [interrupt.Resolution], so the gate runs / denies /
//     runs-with-edited-args accordingly.
//
// The interrupt key is the stable tool name + arguments rather than an
// adapter-generated lifecycle ID. It identifies the same logical call when the
// suspended action is re-entered on resume.
func (t *turnObserver) ApproveToolCall(ctx context.Context, callID, toolName, arguments string, target agentexec.ToolApprovalTarget) agentexec.ToolApprovalVerdict {
	return (&toolGate{controller: t.controller, st: t.st}).ApproveToolCall(ctx, callID, toolName, arguments, target)
}

func (t *toolGate) ApproveToolCall(ctx context.Context, callID, toolName, arguments string, target agentexec.ToolApprovalTarget) agentexec.ToolApprovalVerdict {
	if !t.controller.toolUsesStandardPolicy(toolName) {
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
			Event: hooks.PreToolUse, SessionID: t.st.handle.SessionID, CWD: t.st.cwd,
			Tool: &hooks.ToolInput{Name: toolName, Arguments: arguments},
		})
		hookDecision = approval.HookDecision{
			Block:            dec.Block,
			Reason:           dec.Reason,
			Ask:              dec.Ask,
			RewriteArguments: dec.RewriteArguments,
		}
	}

	mode, err := t.controller.approval.Mode(ctx, t.st.handle.SessionID)
	if err != nil {
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: "approval mode unavailable"}
	}

	plan := approval.ToolCallInput{
		Arguments:    arguments,
		Mode:         mode,
		Hook:         hookDecision,
		SafetyClass:  t.controller.toolSafetyClass(toolName),
		FileMutation: fileMutationScope(target.FileMutations, cmp.Or(hookDecision.RewriteArguments, arguments), t.st.cwd),
		ShellCommand: t.controller.shellCommand(toolName, cmp.Or(hookDecision.RewriteArguments, arguments)),
	}.Plan()
	sessionID := t.st.handle.SessionID
	approvalSubject := ""
	if plan.Action == approval.GatePrompt {
		rememberedArguments, err := tool.ParseArguments(plan.Arguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: validate gated tool %q arguments: %w", toolName, err),
			}
		}
		approvalSubject, err = t.controller.approvalSubject(toolName, rememberedArguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: derive approval subject for tool %q: %w", toolName, err),
			}
		}
		query := approval.Query{SessionID: sessionID, ProjectDir: t.st.cwd, Tool: toolName, Subject: approvalSubject}
		d, ok, err := t.controller.approval.Decide(ctx, query)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: evaluate remembered approval for tool %q: %w", toolName, err),
			}
		}
		autoApproved := false
		// A per-server auto-approve whitelist skips the prompt only after
		// standing rules, so an explicit remembered deny is never overridden.
		if t.controller.mcpToolAutoApproved != nil && target.MCP != (mcpserver.ToolRef{}) {
			autoApproved = t.controller.mcpToolAutoApproved(target.MCP)
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
	if err := t.rememberApproval(ctx, toolName, approvalSubject, res); err != nil {
		return agentexec.ToolApprovalVerdict{Interrupt: err}
	}
	// The human's edited args win over a hook rewrite; fall back to the rewrite
	// when they approved without editing.
	return approvalResolutionVerdict(res, plan.ArgumentOverride)
}

// doomLoopEscalation brakes a model repeating the same call to no effect. It
// raises the ordinary approval interrupt (reusing its resolution lifecycle and
// auto-deny-when-unanswerable machinery — a headless client that cannot answer
// approvals auto-denies, braking the loop automatically) with a reason naming
// the loop. The no-progress streak is reset as the brake fires, so after the
// human's decision the model gets a fresh run of calls before it can trip again;
// on denial it also receives recoverable feedback and must change approach. No
// standing rule is consulted or recorded — this is a one-off brake, not a
// persistent permission.
func (t *toolGate) doomLoopEscalation(ctx context.Context, callID, toolName, arguments string, safetyClass tool.SafetyClass) agentexec.ToolApprovalVerdict {
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

func (t *toolGate) awaitApproval(ctx context.Context, toolName, arguments string, prompt runs.ApprovalPrompt) (interrupt.Resolution, error) {
	pending := runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt}
	if err := pending.Validate(); err != nil {
		return interrupt.Resolution{}, fmt.Errorf("turn: build approval interrupt: %w", err)
	}
	return suspension.Interrupt(ctx, interrupt.InterruptKey(interrupt.Approval.String(), toolName, arguments), pending)
}

func (t *toolGate) rememberApproval(ctx context.Context, toolName, subject string, resolution interrupt.Resolution) error {
	if resolution.RememberScope == "" {
		return nil
	}
	if err := t.controller.approval.Remember(ctx, approval.RememberRequest{
		Scope:      resolution.RememberScope,
		SessionID:  t.st.handle.SessionID,
		ProjectDir: t.st.cwd,
		Tool:       toolName,
		Subject:    subject,
		Decision:   approval.DecisionOf(resolution.Approved),
	}); err != nil {
		return fmt.Errorf("turn: remember approval decision for tool %q: %w", toolName, err)
	}
	return nil
}

func approvalResolutionVerdict(resolution interrupt.Resolution, fallbackArguments string) agentexec.ToolApprovalVerdict {
	if !resolution.Approved {
		return agentexec.ToolApprovalVerdict{Denied: true, DenyReason: denialReason(resolution.Reason)}
	}
	return agentexec.ToolApprovalVerdict{Arguments: cmp.Or(resolution.Arguments, fallbackArguments)}
}

type fileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}

func fileMutationScope(reporter fileMutationReporter, arguments, cwd string) tool.FileMutationScope {
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
func (t *toolGate) resumedToolVerdict(ctx context.Context, toolName string) (agentexec.ToolApprovalVerdict, bool) {
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
	case interrupt.Question:
		return agentexec.ToolApprovalVerdict{Arguments: effectiveArguments}, true
	case interrupt.Approval:
		rememberedArguments, err := tool.ParseArguments(effectiveArguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: validate restored approval tool %q arguments: %w", toolName, err),
			}, true
		}
		approvalSubject, err := t.controller.approvalSubject(toolName, rememberedArguments)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: derive restored approval subject for tool %q: %w", toolName, err),
			}, true
		}
		resolution, err := suspension.DecodeResolution(parked.Response)
		if err != nil {
			return agentexec.ToolApprovalVerdict{
				Interrupt: fmt.Errorf("turn: decode approval resolution: %w", err),
			}, true
		}
		if pending.Approval.Rememberable {
			if err := t.rememberApproval(ctx, toolName, approvalSubject, resolution); err != nil {
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

func (t *turnObserver) OnToolCallStart(process agentexec.ProcessRef, callID, sourceCallID, toolName, arguments string) {
	if !t.projects(process) {
		return
	}
	t.controller.emitProcessEvent(t.st, process, runs.ToolCallStarted{
		CallID:       callID,
		SourceCallID: sourceCallID,
		ToolName:     toolName,
		Arguments:    arguments,
		Activity:     t.controller.toolActivity(toolName, arguments),
		SafetyClass:  t.controller.toolSafetyClass(toolName),
	})
}

func (t *turnObserver) OnToolCallEnd(process agentexec.ProcessRef, callID, toolName, arguments, output string, ref *toolresult.Ref, mutatedPaths []string, err error) {
	if !t.projects(process) {
		// A suspension is an unfinished call, not a tool result. Its logical
		// completion will be observed after the resumed child call returns.
		if hitl.IsSuspended(err) {
			return
		}
		t.postToolHook(toolName, output, err)
		return
	}
	// HITL interrupt: the tool
	// paused for human input. Not a failure — skip the ToolCallFinished
	// event. The turn-park handler drains the in-flight tool item
	// and creates the appropriate interrupt card.
	if hitl.IsSuspended(err) {
		return
	}
	// Feed the doom-loop brake (T13): a completed call — success, error, or a
	// recoverable denial — with the same args and same output as the previous run
	// is a no-progress repeat. The gate reads this count before the next call.
	if !process.Child() {
		t.st.recordToolOutcome(toolName, arguments, output)
	}
	result, outputText := decodeToolResult(t.controller.toolPresenter, toolName, arguments, output)
	end := runs.ToolCallFinished{
		CallID:       callID,
		Arguments:    arguments,
		Result:       result,
		Offload:      ref,
		OutputText:   outputText,
		MutatedPaths: mutatedPaths,
	}
	switch {
	case errors.Is(err, agentexec.ErrToolDenied):
		end.Problem = &transcript.Problem{
			Kind: transcript.DeniedByUserProblem, Scope: transcript.ToolProblem,
		}
	case err != nil:
		end.Problem = &transcript.Problem{
			Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem, Detail: err.Error(),
		}
	}
	t.controller.emitProcessEvent(t.st, process, end)

	projected, projectionErr := t.controller.projectToolOutcome(
		t.st.ctx,
		t.st.handle.SessionID,
		toolName,
		err == nil,
	)
	if projectionErr != nil {
		t.st.span.RecordError(fmt.Errorf("turn: project tool %q outcome: %w", toolName, projectionErr))
	} else if projected != nil {
		t.controller.emitProcessEvent(t.st, process, projected)
	}

	// PostToolUse hooks (observe-only in v1): fire after the result so a user
	// script can audit / notify / integrate. Result-injection isn't plumbed yet
	// — the result already streamed to the model — so the Decision is ignored.
	t.postToolHook(toolName, output, err)
}

func (t *turnObserver) postToolHook(toolName, output string, err error) {
	if !t.controller.toolUsesStandardPolicy(toolName) || t.st.hooks.Empty() {
		return
	}
	_ = t.st.hooks.Run(t.st.ctx, hooks.Input{
		Event: hooks.PostToolUse, SessionID: t.st.handle.SessionID, CWD: t.st.cwd,
		Tool: &hooks.ToolInput{Name: toolName, Result: output}, Reason: errorString(err),
	})
}

func (t *turnObserver) OnMessageDelta(process agentexec.ProcessRef, text string) {
	if !t.projects(process) {
		return
	}
	t.controller.emitProcessEvent(t.st, process, runs.MessageDelta{
		Text: text,
	})
}

// OnReasoningDelta forwards extended-thinking chunks to the turn
// channel as [ReasoningDelta] events. Clients that don't care
// about reasoning can ignore the type in their dispatch switch —
// no event is dropped on the engine side.
func (t *turnObserver) OnReasoningDelta(process agentexec.ProcessRef, text string) {
	if !t.projects(process) {
		return
	}
	t.controller.emitProcessEvent(t.st, process, runs.ReasoningDelta{
		Text: text,
	})
}

// OnUsage forwards the per-round cumulative usage as a [UsageReported] event,
// preserving the live token and cost facts for downstream projection.
// contextTokens is this round's prompt size (the live context occupancy).
func (t *turnObserver) OnUsage(process agentexec.ProcessRef, progress agentexec.UsageProgress) {
	if !t.projects(process) {
		return
	}
	t.controller.emitProcessEvent(t.st, process, runs.UsageReported{
		TokenUsage:    progress.Usage,
		ByModel:       progress.UsageByModel,
		CostUSD:       progress.CostUSD,
		Steps:         progress.Steps,
		ContextTokens: progress.ContextTokens,
	})
}
