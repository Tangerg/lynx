package maintenance

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// titleMaxInputChars caps the slice of the opening user message fed to the
// titler — a title needs the gist, not a whole pasted file, and a shorter
// prompt keeps the (cheap-model) call fast.
const titleMaxInputChars = 4000

// titleMaxRunes bounds the generated title; an over-long model reply is
// truncated rather than rejected (a usable title beats none).
const titleMaxRunes = 80

// Titler generates a short, human-readable session title from a conversation's
// opening user message — the auto-name a client shows in its session list
// instead of an empty entry. Like the [Compactor]'s summary it is a one-shot,
// middleware-free LLM call ([askDirect]) on the (typically cheaper)
// maintenance client. A nil Titler / client makes [Titler.Generate] a no-op.
type Titler struct {
	client *chat.Client
}

// NewTitler builds a Titler over the maintenance chat client.
func NewTitler(client *chat.Client) *Titler {
	return &Titler{client: client}
}

const titlePrompt = `Write a concise title for a conversation that opens with the user message below.

Rules:
- 3 to 6 words, Title Case, at most 60 characters.
- Capture the task/topic; no filler ("Help with", "Question about").
- Output ONLY the title — no quotes, no surrounding punctuation, no markdown, no trailing period.`

// Generate returns a short title derived from firstMessage, or "" (no error)
// when titling isn't possible — a nil receiver/client, an empty message, or a
// model reply that sanitizes to nothing. Best-effort by contract: callers
// leave the session untitled on "" rather than surfacing a failure.
func (t *Titler) Generate(ctx context.Context, firstMessage string) (string, error) {
	if t == nil || t.client == nil {
		return "", nil
	}
	msg := strings.TrimSpace(firstMessage)
	if msg == "" {
		return "", nil
	}
	if len(msg) > titleMaxInputChars {
		msg = msg[:titleMaxInputChars]
	}
	text, err := askDirect(ctx, t.client, titlePrompt, msg)
	if err != nil {
		return "", err
	}
	return sanitizeTitle(text), nil
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
	s = strings.TrimRight(s, ".")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > titleMaxRunes {
		s = strings.TrimSpace(string(r[:titleMaxRunes]))
	}
	return s
}
