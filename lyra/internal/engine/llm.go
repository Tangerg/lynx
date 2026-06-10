package engine

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
)

// llmCallTimeout caps a single LLM call so a hung provider connection
// — no response, or a stream the client can't parse (e.g. the error
// body a no-access model returns on a streaming request) — fails the
// call instead of blocking the run forever. The ctx deadline also lets
// Go's HTTP stack interrupt a stuck tls.Read. This is a hang backstop,
// not a turn budget: keep it generous (use MaxBudget / MaxCostUSD to
// bound normal work).
const llmCallTimeout = 2 * time.Minute

// askDirect runs one synchronous LLM chat call with the supplied
// system + user prompts. Crucially, the call goes through
// [chat.Client.Chat] without any of the platform middleware
// (chat-memory, tools, guardrails) — used by compaction,
// extraction and planning, which work outside the normal
// conversation flow and must not pollute its history.
//
// nil client surfaces as a plain error rather than a panic — a
// defensive guard only, since [New] rejects a nil ChatClient before
// any caller of askDirect can exist.
func askDirect(ctx context.Context, client *chat.Client, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", errors.New("engine: chat client missing")
	}
	ctx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()
	text, _, err := client.Chat().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Call().
		Text(ctx)
	return text, err
}
