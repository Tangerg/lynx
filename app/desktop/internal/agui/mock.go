package agui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
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
type EmitFunc func(Event) error

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
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
type sender func(Event) bool

func makeSender(ctx context.Context, emit EmitFunc) sender {
	return func(e Event) bool {
		if ctx.Err() != nil {
			return false
		}
		return emit(e) == nil
	}
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
	if !send(RunStarted(input.ThreadID, input.RunID)) {
		return
	}

	// 2. Plan snapshot.
	if !send(Custom(customPlan, map[string]any{"items": planItems})) {
		return
	}

	// 3. Replay the user prompt so the demo bubble shows up. Allowed because
	// TEXT_MESSAGE_START.role accepts "user".
	uid := newID("msg")
	if !send(TextMessageStart(uid, "user")) ||
		!send(TextMessageContent(uid, userPrompt)) ||
		!send(TextMessageEnd(uid)) {
		return
	}

	// 4. Open the assistant message. We keep it open across all subsequent
	// content + tool interleavings, and close it before the final running
	// tool — that way the verifier is happy.
	aid := newID("msg")
	if !send(TextMessageStart(aid, "assistant")) {
		return
	}
	if !send(TextMessageContent(aid, introText)) {
		return
	}
	if !send(Custom(customPlanBlock, map[string]any{"messageId": aid})) {
		return
	}
	if !send(TextMessageContent(aid, " "+postPlanText)) {
		return
	}

	if !fireTool(send, aid, "t1") || !fireTool(send, aid, "t2") {
		return
	}
	if !send(TextMessageContent(aid, " "+postGrepText)) {
		return
	}

	// 5. Reasoning — a "thinking" span before the web search.
	rid := newID("reason")
	if !send(ReasoningStart(rid)) {
		return
	}
	if !send(ReasoningMessageStart(rid, aid)) {
		return
	}
	reasoning := "The neverthrow style is closest to what we use elsewhere. " +
		"matklad has the principled argument for why a discriminated union is enough. " +
		"Going to mirror neverthrow's shape but inline it — one fewer dependency."
	for _, chunk := range chunkString(reasoning, 16) {
		if !send(ReasoningMessageContent(rid, chunk)) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Millisecond):
		}
	}
	if !send(ReasoningMessageEnd(rid)) || !send(ReasoningEnd(rid)) {
		return
	}

	// 6. Web search + results.
	if !fireTool(send, aid, "tw") {
		return
	}
	if !send(Custom(customSearchResults, map[string]any{
		"parentMessageId": aid,
		"results":         searchResults,
	})) {
		return
	}

	if !send(TextMessageContent(aid, " "+postSearchText)) {
		return
	}
	if !send(Custom(customCodeProposal, map[string]any{
		"parentMessageId": aid,
		"lang":            "typescript",
		"file":            "src/lib/result.ts",
		"text":            proposedCode,
	})) {
		return
	}
	if !fireTool(send, aid, "t3") {
		return
	}

	if !send(TextMessageContent(aid, " "+postWriteText)) || !fireTool(send, aid, "t4") {
		return
	}
	if !send(TextMessageContent(aid, " "+postEditText)) || !fireTool(send, aid, "t5") {
		return
	}
	if !send(TextMessageContent(aid, " "+postTypecheckText)) || !fireTool(send, aid, "t6") {
		return
	}

	if !send(Custom(customApproval, map[string]any{
		"parentMessageId": aid,
		"text":            "Run integration tests for the auth + billing slice",
		"command":         "pnpm test --filter=auth --filter=billing",
		"reason":          "Tests touch the Stripe sandbox API. Output is logged but no charges are made.",
	})) {
		return
	}

	// 7. Close the assistant message — its cursor stops blinking.
	if !send(TextMessageEnd(aid)) {
		return
	}

	// 8. Final tool stays "running" — no TOOL_CALL_END. The verifier permits
	// us to leave the run open indefinitely (we never send RUN_FINISHED).
	if !send(ToolCallStart("t7", "bash", aid)) || !send(ToolCallArgs("t7", "pnpm typecheck")) {
		return
	}

	// 9. Initial telemetry + periodic ticker.
	initial := telemetry(activityLines[0])
	if !send(Custom(customTelemetry, initial)) {
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
			if !send(Custom(customTelemetry, telemetry(activityLines[idx]))) {
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

	if !send(RunStarted(input.ThreadID, input.RunID)) {
		return
	}

	aid := newID("msg")
	if !send(TextMessageStart(aid, "assistant")) {
		return
	}

	if !streamText(ctx, send, aid,
		"Got it — let me trace where this lives.") {
		return
	}

	// Reasoning span — agent "thinks" about how to approach.
	rid := newID("reason")
	if !send(ReasoningStart(rid)) || !send(ReasoningMessageStart(rid, aid)) {
		return
	}
	if !streamReasoning(ctx, send, rid,
		"The user is asking about something concrete. Best to grep first to "+
			"surface the call sites, then read the most relevant file to "+
			"propose the smallest possible change.") {
		return
	}
	if !send(ReasoningMessageEnd(rid)) || !send(ReasoningEnd(rid)) {
		return
	}

	if !send(TextMessageContent(aid, " I'll search the tree and look at the relevant file:")) {
		return
	}

	// One grep + one read_file so the reply also has tool cards.
	if !fireTool(send, aid, "t2") || !fireTool(send, aid, "t1") {
		return
	}

	if !streamText(ctx, send, aid,
		" Found what I needed — proposing a diff next. Want me to apply it directly, "+
			"or stage it for review first?") {
		return
	}

	if !send(TextMessageEnd(aid)) {
		return
	}
	send(RunFinished(input.ThreadID, input.RunID))
}

// streamText writes `text` to an open assistant message as 3-rune chunks
// every 28ms. Returns false if the client disconnected mid-stream.
func streamText(ctx context.Context, send sender, messageID, text string) bool {
	for _, chunk := range chunkString(text, 3) {
		if !send(TextMessageContent(messageID, chunk)) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(28 * time.Millisecond):
		}
	}
	return true
}

// streamReasoning is the same idea but with bigger chunks + slightly slower
// cadence — reasoning text is meant to feel deliberative, not transcribed.
func streamReasoning(ctx context.Context, send sender, messageID, text string) bool {
	for _, chunk := range chunkString(text, 16) {
		if !send(ReasoningMessageContent(messageID, chunk)) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(30 * time.Millisecond):
		}
	}
	return true
}

// fireTool emits start → args → end for one tool from the script.
func fireTool(send sender, parentMessageID, toolID string) bool {
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
	if !send(ToolCallStart(spec.ID, spec.Fn, parentMessageID)) {
		return false
	}
	if !send(ToolCallArgs(spec.ID, spec.Args)) {
		return false
	}
	return send(ToolCallEnd(spec.ID, ToolCallEndExtras{
		Status:     "ok",
		DurationMs: spec.DurationMs,
		Added:      spec.Added,
		Removed:    spec.Removed,
		Hits:       spec.Hits,
		Lines:      spec.Lines,
	}))
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

