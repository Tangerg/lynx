package server

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// maybeTitleSession auto-names an untitled session from its opening user
// message — fired async off a root run's terminal (never a park). The session
// list then shows a meaningful title instead of an empty entry. Best-effort
// throughout: any miss (no input, already titled, LLM error) just leaves the
// session untitled.
//
// Only root runs (parentRunID == "") open a user turn; a continuation
// (runs.resume) carries no new user input. The untitled check also makes this
// fire only on the FIRST run — once a title lands, later runs see it and skip,
// and a client-supplied title is never overwritten.
func (s *Server) maybeTitleSession(ctx context.Context, sessionID, parentRunID string, userInput []protocol.ContentBlock) {
	if parentRunID != "" {
		return
	}
	prompt := userMessageText(userInput)
	if prompt == "" {
		return
	}
	// Cheap pre-check to skip the LLM call when already titled; the authoritative
	// guard is the atomic RenameIfUntitled below — between this read and that
	// write the title generation runs (an LLM round-trip, seconds), during which
	// the user may rename, so the final write must not clobber unconditionally.
	if sess, err := s.rt.Session().Get(ctx, sessionID); err != nil || strings.TrimSpace(sess.Title) != "" {
		return
	}
	title, err := s.rt.GenerateTitle(ctx, prompt)
	if err != nil || title == "" {
		return
	}
	// Only lands if the session is still untitled — a user rename during
	// generation wins (RenameIfUntitled is a no-op then).
	_ = s.rt.Session().RenameIfUntitled(ctx, sessionID, title)
}

// userMessageText flattens a run's opening user input to plain text for the
// titler — text blocks joined by newlines; image blocks are ignored (a title
// comes from words, not pixels).
func userMessageText(blocks []protocol.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type != protocol.ContentBlockText || blk.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(blk.Text)
	}
	return strings.TrimSpace(b.String())
}
