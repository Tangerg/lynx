package maintenance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
)

// directCallTimeout caps a single maintenance LLM call (compaction
// summary / fact extraction) so a hung provider connection fails the
// call instead of blocking turn-boundary housekeeping forever.
// Independent from the engine's turn-level timeout: this bounds a
// one-shot, middleware-free call, not a full streaming tool-loop, so the
// two evolve for different reasons.
const directCallTimeout = 2 * time.Minute

// askDirect runs one synchronous LLM chat call with the supplied
// system + user prompts. Crucially, the call goes through
// [chat.Client.Chat] without any of the platform middleware
// (chat-memory, tools, guardrails) — compaction and extraction both
// work outside the normal conversation flow and must not pollute its
// history.
//
// nil client surfaces as a plain error rather than a panic — a
// defensive guard only, since the engine rejects a nil ChatClient
// before any of these workers is constructed.
func askDirect(ctx context.Context, client *chat.Client, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", errors.New("maintenance: chat client missing")
	}
	ctx, cancel := context.WithTimeout(ctx, directCallTimeout)
	defer cancel()
	text, _, err := client.Chat().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Call().
		Text(ctx)
	return text, err
}

// renderTranscript flattens messages into a plain-text role-tagged
// transcript a summariser / extractor can read. Lossy by design — tool-call
// arguments and parts are flattened to their text bodies; what we
// need is gist, not fidelity.
func renderTranscript(msgs []chat.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		switch m := msg.(type) {
		case *chat.SystemMessage:
			b.WriteString("[system] ")
			b.WriteString(m.Text)
		case *chat.UserMessage:
			b.WriteString("[user] ")
			b.WriteString(m.Text)
		case *chat.AssistantMessage:
			b.WriteString("[assistant] ")
			b.WriteString(m.JoinedText())
		case *chat.ToolMessage:
			b.WriteString("[tool] ")
			for _, r := range m.ToolReturns {
				if r != nil {
					b.WriteString(r.Result)
					b.WriteString(" ")
				}
			}
		default:
			fmt.Fprintf(&b, "[%s] (unrecognized)", msg.Type())
		}
		b.WriteString("\n")
	}
	return b.String()
}
