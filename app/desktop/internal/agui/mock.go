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

// chunkString returns the string split into pieces of *approximately* n
// bytes, cutting on rune boundaries so multi-byte UTF-8 sequences (em-dash,
// CJK, emoji) never get split mid-codepoint.
func chunkString(s string, n int) []string {
	out := make([]string, 0, len(s)/n+1)
	runes := []rune(s)
	for i := 0; i < len(runes); i += n {
		end := min(i+n, len(runes))
		out = append(out, string(runes[i:end]))
	}
	return out
}

// Run dispatches based on whether the agent has any prior turns.
//
// AbstractAgent accumulates messages locally (both user and assistant turns
// added via TEXT_MESSAGE_START events). So we can't use "is there a user
// message?" as the demo trigger — after the demo plays once, every later
// runAgent() call would still carry the demo's user + assistant messages.
//
// Empty messages == fresh agent == play the demo. Otherwise reply.
func Run(ctx context.Context, input RunAgentInput, emit EmitFunc) {
	if len(input.Messages) == 0 {
		runDemo(ctx, input, emit)
	} else {
		runReply(ctx, input, emit)
	}
}

// send wraps emit so each call checks ctx + returns a bool. Lets the demo
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
// (before a tool call, after a reasoning span ends, before an approval pops)
// so the demo doesn't fire every event back-to-back.
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

// runDemo replays the scripted conversation. The AG-UI verifier requires:
//   - RUN_STARTED to be the very first event
//   - Text-message and tool-call pairs to be properly bracketed
//   - No RUN_FINISHED while text/tool/step is still open
//
// We open ONE assistant message and stream text deltas + interleave tool
// calls inside it, closing it only before the final running tool.
func runDemo(ctx context.Context, input RunAgentInput, emit EmitFunc) {
	send := makeSender(ctx, emit)

	// 1. RUN_STARTED must be first.
	if !send(sdkevents.NewRunStartedEvent(input.ThreadID, input.RunID)) {
		return
	}
	if !pause(ctx, 250, 600) {
		return
	}

	// 2. Plan snapshot.
	if !send(sdkevents.NewCustomEvent(customPlan, sdkevents.WithValue(map[string]any{"items": planItems}))) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	// 3. Replay the user prompt so the demo bubble shows up. Allowed because
	// TEXT_MESSAGE_START.role accepts "user".
	uid := newID("msg")
	if !send(sdkevents.NewTextMessageStartEvent(uid, sdkevents.WithRole("user"))) ||
		!send(sdkevents.NewTextMessageContentEvent(uid, userPrompt)) ||
		!send(sdkevents.NewTextMessageEndEvent(uid)) {
		return
	}
	if !pause(ctx, 500, 1000) {
		return
	}

	// 4. Open the assistant message. We keep it open across all subsequent
	// content + tool interleavings, and close it before the final running
	// tool — that way the verifier is happy.
	aid := newID("msg")
	if !send(sdkevents.NewTextMessageStartEvent(aid, sdkevents.WithRole("assistant"))) {
		return
	}
	if !pause(ctx, 150, 400) {
		return
	}
	if !streamText(ctx, send, aid, introText) {
		return
	}
	if !pause(ctx, 350, 800) {
		return
	}

	if !send(sdkevents.NewCustomEvent(customPlanBlock, sdkevents.WithValue(map[string]any{"messageId": aid}))) {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !streamText(ctx, send, aid, " "+postPlanText) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	if !fireTool(ctx, send, aid, "t1") {
		return
	}
	if !pause(ctx, 250, 650) {
		return
	}
	if !fireTool(ctx, send, aid, "t2") {
		return
	}
	if !pause(ctx, 300, 800) {
		return
	}

	if !streamText(ctx, send, aid, " "+postGrepText) {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	// 5. Reasoning — a "thinking" span before the web search.
	rid := newID("reason")
	if !send(sdkevents.NewReasoningStartEvent(rid)) {
		return
	}
	if !send(&reasoningStart{
		ReasoningMessageStartEvent: sdkevents.NewReasoningMessageStartEvent(rid, "reasoning"),
		ParentMessageID:            aid,
	}) {
		return
	}
	if !pause(ctx, 200, 500) {
		return
	}
	reasoning1 := "The neverthrow style is closest to what we use elsewhere — Ok/Err discriminated " +
		"union, no exotic combinators, no class hierarchy. matklad has the principled argument for " +
		"why a discriminated union is enough and why bigger abstractions like Effect's tagged unions " +
		"are overkill for service-layer code. Going to mirror neverthrow's shape but inline it. " +
		"One fewer dependency, and it makes the type trivially auditable from this file alone."
	if !streamReasoning(ctx, send, rid, reasoning1) {
		return
	}
	if !send(sdkevents.NewReasoningMessageEndEvent(rid)) || !send(sdkevents.NewReasoningEndEvent(rid)) {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	// 6. Web search + results.
	if !fireTool(ctx, send, aid, "tw") {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}
	if !send(sdkevents.NewCustomEvent(customSearchResults, sdkevents.WithValue(map[string]any{
		"parentMessageId": aid,
		"results":         searchResults,
	}))) {
		return
	}
	if !pause(ctx, 500, 1000) {
		return
	}

	if !streamText(ctx, send, aid, " "+postSearchText) {
		return
	}
	if !pause(ctx, 400, 800) {
		return
	}
	if !send(sdkevents.NewCustomEvent(customCodeProposal, sdkevents.WithValue(map[string]any{
		"parentMessageId": aid,
		"lang":            "typescript",
		"file":            "src/lib/result.ts",
		"text":            proposedCode,
	}))) {
		return
	}
	if !pause(ctx, 500, 1000) {
		return
	}

	if !fireTool(ctx, send, aid, "t3") {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	if !streamText(ctx, send, aid, " "+postWriteText) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}
	if !fireTool(ctx, send, aid, "t4") {
		return
	}
	if !pause(ctx, 350, 800) {
		return
	}

	if !streamText(ctx, send, aid, " "+postEditText) {
		return
	}
	if !pause(ctx, 350, 800) {
		return
	}
	if !fireTool(ctx, send, aid, "t5") {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !streamText(ctx, send, aid, " "+postTypecheckText) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}
	if !fireTool(ctx, send, aid, "t6") {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !streamText(ctx, send, aid, " "+postBillingFixText) {
		return
	}
	if !pause(ctx, 500, 1000) {
		return
	}

	// 6b. A second reasoning span before the approval — the agent weighs
	// whether to ask, since sandbox calls are real network traffic.
	rid2 := newID("reason")
	if !send(sdkevents.NewReasoningStartEvent(rid2)) {
		return
	}
	if !send(&reasoningStart{
		ReasoningMessageStartEvent: sdkevents.NewReasoningMessageStartEvent(rid2, "reasoning"),
		ParentMessageID:            aid,
	}) {
		return
	}
	if !pause(ctx, 200, 500) {
		return
	}
	reasoning2 := "Sandbox mode means no actual charges, but the calls still go to api.stripe.com — " +
		"so a misconfigured token or a stale fixture could surface a real error. The auth changes " +
		"are upstream of these calls, so if the unwrap shape is wrong it'll fail loudly. Worth " +
		"asking the user before kicking off a 2-3 minute integration run."
	if !streamReasoning(ctx, send, rid2, reasoning2) {
		return
	}
	if !send(sdkevents.NewReasoningMessageEndEvent(rid2)) || !send(sdkevents.NewReasoningEndEvent(rid2)) {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	if !send(sdkevents.NewCustomEvent(customApproval, sdkevents.WithValue(map[string]any{
		"parentMessageId": aid,
		"text":            "Run integration tests for the auth + billing slice",
		"command":         "pnpm test --filter=auth --filter=billing",
		"reason":          "Tests touch the Stripe sandbox API. Output is logged but no charges are made.",
	}))) {
		return
	}
	if !pause(ctx, 500, 1000) {
		return
	}

	if !streamText(ctx, send, aid, " "+postApprovalText) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	// 7. Close the assistant message — its cursor stops blinking.
	if !send(sdkevents.NewTextMessageEndEvent(aid)) {
		return
	}
	if !pause(ctx, 300, 700) {
		return
	}

	// 8. Final tool stays "running" — no TOOL_CALL_END. The verifier permits
	// us to leave the run open indefinitely (we never send RUN_FINISHED).
	if !send(sdkevents.NewToolCallStartEvent("t7", "bash", sdkevents.WithParentMessageID(aid))) {
		return
	}
	if !pause(ctx, 150, 400) {
		return
	}
	if !streamToolArgs(ctx, send, "t7", "pnpm test --filter=auth --filter=billing") {
		return
	}
	if !pause(ctx, 400, 900) {
		return
	}

	// 9. Initial telemetry + periodic ticker.
	initial := telemetry(activityLines[0])
	if !send(sdkevents.NewCustomEvent(customTelemetry, sdkevents.WithValue(initial))) {
		return
	}
	idx := 0
	ticker := time.NewTicker(2400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idx = (idx + 1) % len(activityLines)
			if !send(sdkevents.NewCustomEvent(customTelemetry, sdkevents.WithValue(telemetry(activityLines[idx])))) {
				return
			}
		}
	}
}

// runReply handles follow-up turns. Not as long as the demo, but it still
// exercises the full event vocabulary (text, reasoning, tool calls) so the
// UI keeps something interesting to render after the user types.
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

	// Reasoning span — agent "thinks" about how to approach.
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
			"the most relevant file to ground the answer. If the call graph is shallow I can "+
			"propose a direct fix; if it's deeper I should explain the structure before suggesting "+
			"changes so they can push back if my mental model is wrong.") {
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

	// One grep + one read_file so the reply also has tool cards.
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
		" Found what I needed — proposing a diff next. The change is small (about 8 lines net) "+
			"and stays inside the same file. Want me to apply it directly, or stage it for review first?") {
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

// fireTool emits start → streaming args → end for one tool from the script.
// The end event carries our demo summary fields (status, duration, line
// counts) via the toolCallEnd wrapper.
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
	// Brief "execution" pause proportional to the tool's claimed duration —
	// capped so even a 2.4s typecheck doesn't stall the demo too long.
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

// telemetry builds the periodic telemetry payload — values are static except
// the activity line, which rotates.
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
