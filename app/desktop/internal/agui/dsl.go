package agui

import (
	"context"
	"time"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// env carries the live state a Step needs to emit events. One env per run,
// mutated by steps as the script progresses (notably `assistantID` —
// the currently-open assistant text message, opened lazily by the first
// Say() and closed by CloseAssistant() or the implicit end-of-script).
type env struct {
	ctx         context.Context
	input       RunAgentInput
	send        sender
	assistantID string
}

// Step is one unit of demo script. Returning false aborts the run
// (client disconnect, ctx cancellation, etc.).
type Step interface {
	Run(e *env) bool
}

// runScript walks a script start-to-finish with the standard RUN_STARTED /
// (optional RUN_FINISHED) bracketing. Steps that need long-running
// behaviour (Telemetry) can either be last or run until the client
// disconnects — runScript returns either way.
func runScript(ctx context.Context, input RunAgentInput, emit EmitFunc, script []Step) {
	send := makeSender(ctx, emit)
	if !send(sdkevents.NewRunStartedEvent(input.ThreadID, input.RunID)) {
		return
	}
	e := &env{ctx: ctx, input: input, send: send}
	for _, step := range script {
		if !step.Run(e) {
			return
		}
	}
	// Close any open assistant message before signalling the run is done.
	if e.assistantID != "" {
		send(sdkevents.NewTextMessageEndEvent(e.assistantID))
		e.assistantID = ""
	}
	send(sdkevents.NewRunFinishedEvent(input.ThreadID, input.RunID))
}

// ---------------------------------------------------------------------------
// Step primitives
// ---------------------------------------------------------------------------

// Pause sleeps for a random duration in [minMs, maxMs] ms.
func Pause(minMs, maxMs int) Step { return pauseStep{minMs, maxMs} }

type pauseStep struct{ minMs, maxMs int }

func (s pauseStep) Run(e *env) bool { return pause(e.ctx, s.minMs, s.maxMs) }

// User replays a user turn (TextMessageStart/Content/End) as one synchronous
// burst — the user already saw the text they typed, so we don't smooth it.
func User(text string) Step { return userStep{text} }

type userStep struct{ text string }

func (s userStep) Run(e *env) bool {
	uid := newID("msg")
	return e.send(sdkevents.NewTextMessageStartEvent(uid, sdkevents.WithRole("user"))) &&
		e.send(sdkevents.NewTextMessageContentEvent(uid, s.text)) &&
		e.send(sdkevents.NewTextMessageEndEvent(uid))
}

// Say streams text into the assistant message, opening it lazily on first
// call. Repeated Say() calls append to the same open message; insert
// CloseAssistant() (or a non-text step that closes implicitly) between
// chunks if the script needs to break the message.
func Say(text string) Step { return sayStep{text} }

type sayStep struct{ text string }

func (s sayStep) Run(e *env) bool {
	if e.assistantID == "" {
		e.assistantID = newID("msg")
		if !e.send(sdkevents.NewTextMessageStartEvent(e.assistantID, sdkevents.WithRole("assistant"))) {
			return false
		}
		if !pause(e.ctx, 120, 350) {
			return false
		}
	}
	return streamText(e.ctx, e.send, e.assistantID, s.text)
}

// CloseAssistant finalises the currently-open assistant message (no-op if
// nothing is open).
func CloseAssistant() Step { return closeAssistantStep{} }

type closeAssistantStep struct{}

func (closeAssistantStep) Run(e *env) bool {
	if e.assistantID == "" {
		return true
	}
	ok := e.send(sdkevents.NewTextMessageEndEvent(e.assistantID))
	e.assistantID = ""
	return ok
}

// Think emits a reasoning span attached to the current assistant message.
// If no assistant message is open the reasoning still streams but with no
// parentMessageId, so the UI shows it as a free-standing span.
func Think(text string) Step { return thinkStep{text} }

type thinkStep struct{ text string }

func (s thinkStep) Run(e *env) bool {
	rid := newID("reason")
	if !e.send(sdkevents.NewReasoningStartEvent(rid)) {
		return false
	}
	if !e.send(&reasoningStart{
		ReasoningMessageStartEvent: sdkevents.NewReasoningMessageStartEvent(rid, "reasoning"),
		ParentMessageID:            e.assistantID,
	}) {
		return false
	}
	if !pause(e.ctx, 150, 400) {
		return false
	}
	if !streamReasoning(e.ctx, e.send, rid, s.text) {
		return false
	}
	return e.send(sdkevents.NewReasoningMessageEndEvent(rid)) &&
		e.send(sdkevents.NewReasoningEndEvent(rid))
}

// RunTool fires a scripted tool from `toolScript` by id. Use this for
// canned demo tools (read_file, grep, web_search, etc.) that already have
// args + duration + summary set up in mock_script.go.
func RunTool(id string) Step { return toolStep{id} }

type toolStep struct{ id string }

func (s toolStep) Run(e *env) bool {
	return fireTool(e.ctx, e.send, e.assistantID, s.id)
}

// Tool fires an ad-hoc tool call inline — for demos that don't want to
// add another entry to the shared toolScript table. `args` streams as
// the TOOL_CALL_ARGS body.
func Tool(name, args string, summary ToolSummary) Step {
	return adHocToolStep{name: name, args: args, summary: summary}
}

// ToolSummary mirrors the demo-summary fields on the toolCallEnd
// extension (status / durationMs / added / removed / hits / lines).
type ToolSummary struct {
	Status     string
	DurationMs int
	Added      *int
	Removed    *int
	Hits       *int
	Lines      *int
}

type adHocToolStep struct {
	name    string
	args    string
	summary ToolSummary
}

func (s adHocToolStep) Run(e *env) bool {
	id := newID("tool")
	if !e.send(sdkevents.NewToolCallStartEvent(id, s.name, sdkevents.WithParentMessageID(e.assistantID))) {
		return false
	}
	if !pause(e.ctx, 80, 220) {
		return false
	}
	if !streamToolArgs(e.ctx, e.send, id, s.args) {
		return false
	}
	execMin, execMax := toolExecPauseRange(s.summary.DurationMs)
	if !pause(e.ctx, execMin, execMax) {
		return false
	}
	status := s.summary.Status
	if status == "" {
		status = "ok"
	}
	return e.send(&toolCallEnd{
		ToolCallEndEvent: sdkevents.NewToolCallEndEvent(id),
		Status:           status,
		DurationMs:       s.summary.DurationMs,
		Added:            s.summary.Added,
		Removed:          s.summary.Removed,
		Hits:             s.summary.Hits,
		Lines:            s.summary.Lines,
	})
}

// CustomFn emits a CUSTOM event whose value is built from the live env —
// `assistantID`, `input`, anything the step needs to interpolate at
// runtime. Used for plan-block / search-results / code-proposal etc.
// where the value depends on the currently-open assistant message.
func CustomFn(name string, build func(e *env) any) Step {
	return customStep{name, build}
}

// CustomVal is the static-value sugar over CustomFn — passes a constant
// builder. Kept as a separate top-level helper so the most common case
// (no env access) stays terse at the call site.
func CustomVal(name string, value any) Step {
	return CustomFn(name, func(*env) any { return value })
}

type customStep struct {
	name  string
	build func(e *env) any
}

func (s customStep) Run(e *env) bool {
	return e.send(sdkevents.NewCustomEvent(s.name, sdkevents.WithValue(s.build(e))))
}

// Approval emits an approval request CUSTOM event and BLOCKS until the
// user clicks Approve / Decline on the rendered card (or the client
// disconnects). The decision is forwarded to a follow-up event so the
// frontend can mark the card as decided.
//
// Wire shape — the `lyra.approval` event carries:
//   { requestId, parentMessageId, text, command, reason }
// The frontend renders an ApprovalCard, the user clicks; we get a POST
// /permission with { requestId, decision }, and unblock here.
//
// Follow-up `lyra.approval-result` event:
//   { requestId, decision: "approved" | "declined" }
// The reducer updates the original block in-place.
func Approval(text, command, reason string) Step {
	return approvalStep{text: text, command: command, reason: reason}
}

type approvalStep struct {
	text, command, reason string
}

func (s approvalStep) Run(e *env) bool {
	id := newID("approval")

	if !e.send(sdkevents.NewCustomEvent(customApproval, sdkevents.WithValue(map[string]any{
		"requestId":       id,
		"parentMessageId": e.assistantID,
		"text":            s.text,
		"command":         s.command,
		"reason":          s.reason,
	}))) {
		return false
	}

	ch := permissions.register(id)
	defer permissions.release(id)

	select {
	case <-e.ctx.Done():
		return false
	case resp := <-ch:
		// Tell the frontend the card is now decided. The reducer
		// looks up the block by requestId and stamps the decision.
		if !e.send(sdkevents.NewCustomEvent(customApprovalResult, sdkevents.WithValue(map[string]any{
			"requestId": id,
			"decision":  string(resp.Decision),
		}))) {
			return false
		}
		// If declined, the script just continues — current demos
		// don't branch on decline. Real LLM scripts would inspect
		// the decision and abort the tool execution.
		return true
	}
}

// TelemetryLoop emits an initial telemetry payload then rotates through
// `activities` every 2.4s until the client disconnects. Should be the
// LAST step in a script — it runs forever.
func TelemetryLoop(activities []string) Step { return telemetryStep{activities} }

type telemetryStep struct {
	activities []string
}

func (s telemetryStep) Run(e *env) bool {
	if len(s.activities) == 0 {
		return true
	}
	if !e.send(sdkevents.NewCustomEvent(customTelemetry, sdkevents.WithValue(telemetry(s.activities[0])))) {
		return false
	}
	idx := 0
	ticker := time.NewTicker(2400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return false
		case <-ticker.C:
			idx = (idx + 1) % len(s.activities)
			if !e.send(sdkevents.NewCustomEvent(customTelemetry, sdkevents.WithValue(telemetry(s.activities[idx])))) {
				return false
			}
		}
	}
}
