package engine

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// askDirect runs one synchronous LLM chat call with the supplied
// system + user prompts. Crucially, the call goes through
// [chat.Client.Chat] without any of the platform middleware
// (chat-memory, tools, guardrails) — used by compaction,
// extraction and planning, which work outside the normal
// conversation flow and must not pollute its history.
//
// nil client surfaces as a plain error rather than a panic so the
// engine's lazy-init paths (Compactor / Extractor / Planner all
// degrade to no-op when ChatClient is missing) can detect the
// condition.
func askDirect(ctx context.Context, client *chat.Client, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", errors.New("engine: chat client missing")
	}
	text, _, err := client.Chat().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Call().
		Text(ctx)
	return text, err
}
