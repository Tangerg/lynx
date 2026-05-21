package agui

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"math/rand/v2"
	"time"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// RunAgentInput — body posted to /run. Only the fields we use are decoded;
// the rest is ignored (passthrough is fine).
type RunAgentInput struct {
	ThreadID string          `json:"threadId"`
	RunID    string          `json:"runId"`
	Messages []ClientMessage `json:"messages"`
}

type ClientMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// EmitFunc reports an event to the HTTP handler. Returns an error if the
// connection has dropped — the runner uses that as a stop signal.
type EmitFunc func(sdkevents.Event) error

func newID(prefix string) string {
	var b [4]byte
	_, _ = cryptorand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

// Run dispatches based on whether the agent has any prior turns.
//
// AbstractAgent accumulates messages locally (both user and assistant turns
// added via TEXT_MESSAGE_START events). So we can't use "is there a user
// message?" as the demo trigger — after the demo plays once, every later
// runAgent() call would still carry the demo's user + assistant messages.
//
// Empty messages == fresh agent == play the demo for the active session.
// Non-empty == follow-up turn → reply with the canned response.
func Run(ctx context.Context, input RunAgentInput, emit EmitFunc) {
	if len(input.Messages) == 0 {
		runScript(ctx, input, emit, resolveDemo(input.ThreadID))
		return
	}
	runReply(ctx, input, emit)
}

// ---------------------------------------------------------------------------
// Sender / timing primitives — used by both the script runner (dsl.go) and
// the canned reply path below.
// ---------------------------------------------------------------------------

// sender wraps emit so each call checks ctx + returns a bool. Lets scripts
// fail-fast cleanly when the client disconnects.
type sender func(sdkevents.Event) bool

func makeSender(ctx context.Context, emit EmitFunc) sender {
	return func(e sdkevents.Event) bool {
		if ctx.Err() != nil {
			return false
		}
		return emit(e) == nil
	}
}

// pause sleeps for a uniformly-random duration in [minMs, maxMs] ms.
// Returns false if ctx is canceled mid-wait. Use it between major beats
// (before a tool call, after a reasoning span ends, before an approval
// pops) so the demo doesn't fire every event back-to-back.
func pause(ctx context.Context, minMs, maxMs int) bool {
	if maxMs < minMs {
		minMs, maxMs = maxMs, minMs
	}
	d := time.Duration(minMs+rand.IntN(maxMs-minMs+1)) * time.Millisecond
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// streamText emits `text` as token-sized chunks (4-9 runes) with jittered
// cadence (40-100ms per chunk), targeting ~110 chars/sec. Roughly half the
// speed of a fast LLM stream so the fade-in animation has time to register
// per word.
func streamText(ctx context.Context, send sender, messageID, text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); {
		n := 4 + rand.IntN(6) // 4..9
		end := min(i+n, len(runes))
		if !send(sdkevents.NewTextMessageContentEvent(messageID, string(runes[i:end]))) {
			return false
		}
		i = end
		d := time.Duration(40+rand.IntN(61)) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
		}
	}
	return true
}

// streamReasoning streams a reasoning body with larger chunks (10-18 runes)
// at 50-110ms — reasoning is meant to feel deliberative, not transcribed,
// so even slower than text streaming.
func streamReasoning(ctx context.Context, send sender, messageID, text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); {
		n := 10 + rand.IntN(9) // 10..18
		end := min(i+n, len(runes))
		if !send(sdkevents.NewReasoningMessageContentEvent(messageID, string(runes[i:end]))) {
			return false
		}
		i = end
		d := time.Duration(50+rand.IntN(61)) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
		}
	}
	return true
}

// streamToolArgs splits the argument string into 3-7 rune chunks with
// 30-75ms gaps. Tool args render in a small inline card so they don't
// need to feel as fluid as the main message stream.
func streamToolArgs(ctx context.Context, send sender, toolID, args string) bool {
	runes := []rune(args)
	if len(runes) == 0 {
		return true
	}
	for i := 0; i < len(runes); {
		n := 3 + rand.IntN(5) // 3..7
		end := min(i+n, len(runes))
		if !send(sdkevents.NewToolCallArgsEvent(toolID, string(runes[i:end]))) {
			return false
		}
		i = end
		d := time.Duration(30+rand.IntN(46)) * time.Millisecond
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
		}
	}
	return true
}

// fireTool runs a scripted tool by id (from toolScript in mock_script.go)
// against the current assistant message. Used by RunTool() in dsl.go.
func fireTool(ctx context.Context, send sender, parentMessageID, toolID string) bool {
	var spec *toolSpec
	for i := range toolScript {
		if toolScript[i].ID == toolID {
			spec = &toolScript[i]
			break
		}
	}
	if spec == nil {
		return true
	}
	if !send(sdkevents.NewToolCallStartEvent(spec.ID, spec.Fn, sdkevents.WithParentMessageID(parentMessageID))) {
		return false
	}
	if !pause(ctx, 80, 220) {
		return false
	}
	if !streamToolArgs(ctx, send, spec.ID, spec.Args) {
		return false
	}
	execMin := 120 + spec.DurationMs/8
	execMax := execMin + 250
	if execMax > 900 {
		execMax = 900
	}
	if execMin > execMax {
		execMin = execMax - 100
	}
	if !pause(ctx, execMin, execMax) {
		return false
	}
	return send(&toolCallEnd{
		ToolCallEndEvent: sdkevents.NewToolCallEndEvent(spec.ID),
		Status:           "ok",
		DurationMs:       spec.DurationMs,
		Added:            spec.Added,
		Removed:          spec.Removed,
		Hits:             spec.Hits,
		Lines:            spec.Lines,
	})
}

// telemetry builds the periodic telemetry payload — values are static
// except the activity line, which rotates.
func telemetry(activity string) map[string]any {
	return map[string]any{
		"step":       5,
		"totalSteps": 7,
		"activity":   activity,
		"tokens":     map[string]string{"used": "47.2k", "total": "200k"},
		"ctxPct":     24,
		"cost":       "0.34",
	}
}

// ---------------------------------------------------------------------------
// Follow-up reply (any session, any text the user types)
// ---------------------------------------------------------------------------

// runReply handles follow-up turns. Same vocabulary as the demo script
// (text + reasoning + a couple of tools) so the chat keeps something
// interesting to render after the user types.
func runReply(ctx context.Context, input RunAgentInput, emit EmitFunc) {
	send := makeSender(ctx, emit)

	if !send(sdkevents.NewRunStartedEvent(input.ThreadID, input.RunID)) {
		return
	}
	if !pause(ctx, 200, 500) {
		return
	}

	aid := newID("msg")
	if !send(sdkevents.NewTextMessageStartEvent(aid, sdkevents.WithRole("assistant"))) {
		return
	}
	if !pause(ctx, 150, 400) {
		return
	}

	if !streamText(ctx, send, aid,
		"Got it — let me trace where this lives. I'll grep the tree first to find the call sites, "+
			"then read the most relevant file so I can propose the smallest possible change.") {
		return
	}
	if !pause(ctx, 350, 800) {
		return
	}

	rid := newID("reason")
	if !send(sdkevents.NewReasoningStartEvent(rid)) ||
		!send(&reasoningStart{
			ReasoningMessageStartEvent: sdkevents.NewReasoningMessageStartEvent(rid, "reasoning"),
			ParentMessageID:            aid,
		}) {
		return
	}
	if !pause(ctx, 200, 500) {
		return
	}
	if !streamReasoning(ctx, send, rid,
		"The user is asking about something concrete — a specific behavior or symbol they've "+
			"already noticed. Best move is to grep first to surface the call sites, then read "+
			"the most relevant file to ground the answer.") {
		return
	}
	if !send(sdkevents.NewReasoningMessageEndEvent(rid)) || !send(sdkevents.NewReasoningEndEvent(rid)) {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !streamText(ctx, send, aid, " Searching the tree and pulling the relevant file:") {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	if !fireTool(ctx, send, aid, "t2") {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}
	if !fireTool(ctx, send, aid, "t1") {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !streamText(ctx, send, aid,
		" Found what I needed — proposing a diff next. Want me to apply it directly, "+
			"or stage it for review first?") {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	if !send(sdkevents.NewTextMessageEndEvent(aid)) {
		return
	}
	send(sdkevents.NewRunFinishedEvent(input.ThreadID, input.RunID))
}
