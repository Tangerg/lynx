package maintenance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// ClientFunc resolves the chat client the maintenance services run on. It is
// read per call — not captured once at construction — so a runtime change to
// the utility model (models.setUtilityRole) takes effect at the next turn
// boundary. The runtime's implementation never returns nil (it falls back to
// the main turn client); a nil ClientFunc, or one that returns nil, leaves the
// owning service unable to call and surfaces as [askDirect]'s missing-client
// error (or a no-op, for the best-effort [Titler]).
type ClientFunc func(context.Context) *chatclient.Client

// directCallTimeout caps a single maintenance LLM call (compaction
// summary / fact extraction) so a hung provider connection fails the
// call instead of blocking turn-boundary housekeeping forever.
// Independent from the kernel's turn-level timeout: this bounds a
// one-shot, middleware-free call, not a full streaming tool-loop, so the
// two evolve for different reasons.
const directCallTimeout = 2 * time.Minute

// askDirect runs one synchronous LLM chat call with the supplied
// system + user prompts. Crucially, the call goes through
// [chatclient.Client.Call] without any of the platform middleware
// (chat history, tools, guardrails) — compaction and extraction both
// work outside the normal conversation flow and must not pollute its
// history.
//
// nil client surfaces as a plain error rather than a panic — a
// defensive guard only, since the kernel rejects a nil ChatClient
// before any of these workers is constructed.
func askDirect(ctx context.Context, client *chatclient.Client, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", errors.New("maintenance: chat client missing")
	}
	ctx, cancel := context.WithTimeout(ctx, directCallTimeout)
	defer cancel()
	request := &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(systemPrompt),
		chat.NewUserMessage(chat.NewTextPart(userPrompt)),
	}}
	response, err := client.Call(ctx, request)
	if err != nil {
		return "", err
	}
	return response.Text(), nil
}

// uncappedToolResults is the [renderTranscript] toolResultCap that leaves tool
// bodies intact — used by the trigger estimate and the fact extractor, which
// must see the real footprint / full content (only the summariser caps).
const uncappedToolResults = 0

// renderTranscript flattens messages into a plain-text role-tagged
// transcript a summariser / extractor can read. Lossy by design — tool-call
// arguments and parts are flattened to their text bodies; what we
// need is gist, not fidelity.
//
// toolResultCap > 0 truncates each tool-result body to that many bytes (head +
// tail, with the elision marked); 0 leaves bodies intact. The summariser passes
// a cap so a few large tool outputs (the very thing the token trigger fires on)
// don't dominate its own input; the trigger estimate and the fact extractor
// pass 0 because they must see the real footprint / full content.
func renderTranscript(msgs []chat.Message, toolResultCap int) string {
	var b strings.Builder
	for _, msg := range msgs {
		switch msg.Role {
		case chat.RoleSystem:
			b.WriteString("[system] ")
			b.WriteString(msg.Text())
		case chat.RoleUser:
			b.WriteString("[user] ")
			b.WriteString(msg.Text())
		case chat.RoleAssistant:
			b.WriteString("[assistant] ")
			b.WriteString(msg.Text())
		case chat.RoleTool:
			b.WriteString("[tool] ")
			for _, part := range msg.Parts {
				if part.Kind == chat.PartToolResult && part.ToolResult != nil {
					b.WriteString(capText(part.ToolResult.Result, toolResultCap))
					b.WriteString(" ")
				}
			}
		default:
			fmt.Fprintf(&b, "[%s] (unrecognized)", msg.Role)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// headTail reduces s to a head (¾) + tail (¼) preview when it exceeds limit,
// the elided middle replaced by marker(elidedChars). Cuts snap to rune
// boundaries so a multibyte rune is never split. Returns s unchanged and false
// when limit <= 0 or s already fits. Shared by the summariser's input cap
// ([capText]) and the compaction ladder's stored trim (compaction_ladder.go);
// each supplies its own marker.
func headTail(s string, limit int, marker func(elided int) string) (string, bool) {
	if limit <= 0 || len(s) <= limit {
		return s, false
	}
	head, tailStart := limit*3/4, len(s)-limit/4
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	return s[:head] + marker(tailStart-head) + s[tailStart:], true
}

// capText bounds an oversized body for the SUMMARISER'S INPUT (transient, not
// stored) — head+tail with the elided middle marked. limit <= 0 or an
// already-small body is returned unchanged.
func capText(s string, limit int) string {
	out, _ := headTail(s, limit, func(elided int) string {
		return fmt.Sprintf("\n…[%d bytes elided for summary]…\n", elided)
	})
	return out
}
