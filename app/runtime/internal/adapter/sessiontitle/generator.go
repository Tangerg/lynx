// Package sessiontitle generates a best-effort Session title through the
// utility-model capability without exposing model clients to Application.
package sessiontitle

import (
	"context"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/utilitymodel"
)

// titleMaxInputRunes caps the slice of the opening user message fed to the
// generator. A title needs the gist, not a whole pasted file, and a shorter
// prompt keeps the (cheap-model) call fast.
const titleMaxInputRunes = 4000

const (
	titleModelInputBytes   = 20 * 1024
	titleModelOutputTokens = int64(64)
)

// titleMaxRunes bounds the generated title; an over-long model reply is
// truncated rather than rejected (a usable title beats none).
const titleMaxRunes = 80

// Generator produces a short, human-readable Session title from the opening
// user message. It uses one middleware-free call to the typically cheaper
// utility model. A nil Generator or Resolver makes [Generator.Generate] a
// no-op.
type Generator struct {
	client utilitymodel.Resolver
}

// NewGenerator builds a Generator over a per-call chat-client resolver.
func NewGenerator(client utilitymodel.Resolver) *Generator {
	return &Generator{client: client}
}

const titlePrompt = `Write a concise title for a conversation that opens with the user message below.

Rules:
- 3 to 6 words, Title Case, at most 60 characters.
- Capture the task/topic; no filler ("Help with", "Question about").
- Output ONLY the title — no quotes, no surrounding punctuation, no markdown, no trailing period.`

// Generate returns a short title derived from firstMessage. The sanitized first
// line is the deterministic fallback when the utility model is unavailable or
// produces no title. A provider failure returns that usable fallback together
// with the error so the caller can persist stable Session identity while still
// recording the degraded maintenance attempt.
func (g *Generator) Generate(ctx context.Context, firstMessage string) (string, error) {
	msg := strings.TrimSpace(firstMessage)
	if msg == "" {
		return "", nil
	}
	fallback := sanitizeTitle(msg)
	if g == nil || g.client == nil {
		return fallback, nil
	}
	if runes := []rune(msg); len(runes) > titleMaxInputRunes {
		msg = string(runes[:titleMaxInputRunes])
	}
	client := g.client(ctx)
	if client == nil {
		return fallback, nil
	}
	text, err := utilitymodel.Complete(ctx, client, utilitymodel.Prompt{
		SystemPrompt: titlePrompt, UserPrompt: msg,
		MaxInputBytes: titleModelInputBytes, MaxOutputTokens: titleModelOutputTokens,
	})
	if err != nil {
		return fallback, err
	}
	title := sanitizeTitle(text)
	if title == "" {
		return fallback, nil
	}
	return title, nil
}

// sanitizeTitle trims a model reply to a clean single-line title: first
// non-empty line only, surrounding quotes + trailing period stripped, capped
// at titleMaxRunes (on a rune boundary, never mid-codepoint).
func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if before, _, found := strings.Cut(s, "\n"); found {
		s = strings.TrimSpace(before)
	}
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".。!?！？")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > titleMaxRunes {
		s = strings.TrimSpace(string(r[:titleMaxRunes]))
	}
	return s
}
