package agui

import (
	"context"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// runReply handles every follow-up turn — anything after the demo has
// played its initial script. Same vocabulary as the demo scripts (text +
// reasoning + a couple of tools) so the chat keeps something interesting
// to render after the user types.
//
// Single canned response for now; if/when we want per-demo follow-ups
// the right move is to extend the Step DSL with a `Reply` step kind and
// let each demo declare its own reply script.
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
	if !send(sdkevents.NewReasoningMessageEndEvent(rid)) ||
		!send(sdkevents.NewReasoningEndEvent(rid)) {
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
