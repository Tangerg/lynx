package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/core/chat"
)

// ExportSession serializes a session to a portable artifact (AUX_API §4.3).
// format=json (default) produces a round-trippable SessionArtifact —
// metadata + chat history + canonical items + runs — that ImportSession
// restores. format=md produces a human-readable transcript (not re-importable).
// Returned inline: lyra is a local loopback runtime, so there's no out-of-band
// file channel nor a giant-payload concern.
func (s *Server) ExportSession(ctx context.Context, in protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	snapshot, err := s.sessions.ReadSnapshot(ctx, s.coordinator, in.SessionID)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, in.SessionID)
		}
		return nil, wireSessionErr(err)
	}

	format := in.Format
	if format == "" {
		format = protocol.ExportFormatJSON
	}

	switch format {
	case protocol.ExportFormatMarkdown:
		return &protocol.ExportSessionResponse{
			Format:   format,
			Markdown: renderSessionMarkdown(s.sessionToWire(snapshot.Session, protocol.SessionStatusIdle), snapshot.Items),
		}, nil
	case protocol.ExportFormatJSON:
	default:
		return nil, fmt.Errorf("%w: unsupported export format %q", protocol.ErrInvalidParams, format)
	}

	msgBlobs := make([]json.RawMessage, 0, len(snapshot.Messages))
	for _, m := range snapshot.Messages {
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("sessions.export: marshal message: %w", err)
		}
		msgBlobs = append(msgBlobs, b)
	}

	artRuns := make([]protocol.ArtifactRun, 0, len(snapshot.Runs))
	for _, r := range snapshot.Runs {
		artRuns = append(artRuns, protocol.ArtifactRun{
			UpdatedAt: r.UpdatedAt, MessageMark: r.MessageMark, Run: presentRun(r),
		})
	}
	artItems := make([]protocol.ArtifactItem, 0, len(snapshot.Items))
	for _, it := range snapshot.Items {
		artItems = append(artItems, protocol.ArtifactItem{Item: presentItem(it)})
	}

	return &protocol.ExportSessionResponse{
		Format: format,
		Artifact: &protocol.SessionArtifact{
			Version:  protocol.SessionArtifactVersion,
			Session:  s.sessionToWire(snapshot.Session, protocol.SessionStatusIdle),
			Messages: msgBlobs,
			Runs:     artRuns,
			Items:    artItems,
		},
	}, nil
}

// ImportSession recreates a session from a SessionArtifact under its ORIGINAL
// id (restore semantics): it upserts the session record, replaces any existing
// history, then re-seeds the chat messages and re-persists the canonical items
// and runs. Re-importing the same artifact is idempotent; importing over an
// existing session restores it.
func (s *Server) ImportSession(ctx context.Context, in protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
	art := in.Artifact
	if art.Version != protocol.SessionArtifactVersion {
		return nil, fmt.Errorf("%w: unsupported artifact version %d (want %d)", protocol.ErrInvalidParams, art.Version, protocol.SessionArtifactVersion)
	}
	if art.Session.ID == "" {
		return nil, fmt.Errorf("%w: artifact.session.id is required", protocol.ErrInvalidParams)
	}

	// Decode the chat messages up front so a malformed artifact fails before we
	// mutate any storage.
	msgs := make([]chat.Message, 0, len(art.Messages))
	for i, blob := range art.Messages {
		var m chat.Message
		if err := json.Unmarshal(blob, &m); err != nil {
			return nil, fmt.Errorf("%w: artifact.messages[%d]: %w", protocol.ErrInvalidParams, i, err)
		}
		msgs = append(msgs, m)
	}
	runs, items, err := canonicalArtifact(art, len(msgs))
	if err != nil {
		return nil, err
	}

	id := art.Session.ID

	// Hand the strictly-decoded canonical aggregate to the lifecycle coordinator.
	// It commits the whole thing as ONE transaction — upsert the
	// session row, replace existing history (drop old items/runs + clear the
	// chat log + stale open interrupts), re-seed the messages, re-persist
	// runs+items — so a mid-sequence failure after the destructive
	// delete/truncate can't leave the session row live but its history
	// half-destroyed (an import-over losing the prior history with nothing to
	// replace it).
	if err := s.sessions.RestoreSession(ctx, s.coordinator, artifactToSession(art.Session), msgs, runs, items); err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, id)
		}
		if errors.Is(err, transcript.ErrIdentityConflict) {
			return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
		}
		return nil, wireSessionErr(err)
	}

	ses, err := s.sessions.Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &protocol.ImportSessionResponse{Session: &out}, nil
}

// artifactToSession maps the wire Session carried in an artifact back to the
// domain session. The wire shape omits the
// delegation-lineage fields (Kind / ParentID); a restored session is a
// standalone user-facing conversation, so Kind/ParentID stay empty.
func artifactToSession(w protocol.Session) session.Session {
	return session.Session{
		ID:        w.ID,
		Title:     w.Title,
		Cwd:       w.Cwd,
		Model:     w.Model,
		StartedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
		Metadata:  w.Metadata,
	}
}

// renderSessionMarkdown produces a human-readable transcript of a session — a
// header plus each canonical item rendered by type. It is not re-importable
// (use format=json for that).
func renderSessionMarkdown(ses protocol.Session, items []transcript.Item) string {
	var b strings.Builder
	title := ses.Title
	if title == "" {
		title = ses.ID
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if ses.Cwd != "" {
		fmt.Fprintf(&b, "- cwd: `%s`\n", ses.Cwd)
	}
	if ses.Model != "" {
		fmt.Fprintf(&b, "- model: `%s`\n", ses.Model)
	}
	b.WriteString("\n")

	for _, raw := range items {
		it := presentItem(raw)
		switch it.Type {
		case protocol.ItemTypeUserMessage:
			fmt.Fprintf(&b, "## User\n\n%s\n\n", contentText(it))
		case protocol.ItemTypeAgentMessage:
			fmt.Fprintf(&b, "## Assistant\n\n%s\n\n", contentText(it))
		case protocol.ItemTypeReasoning:
			if !it.Redacted && it.Text != "" {
				fmt.Fprintf(&b, "> _(reasoning)_ %s\n\n", it.Text)
			}
		case protocol.ItemTypeToolCall:
			if it.Tool != nil {
				fmt.Fprintf(&b, "→ **tool** `%s`\n\n", it.Tool.Name)
			}
		case protocol.ItemTypePlan:
			b.WriteString("**Plan:**\n")
			for _, st := range it.Steps {
				fmt.Fprintf(&b, "- [%s] %s\n", st.Status, st.Title)
			}
			b.WriteString("\n")
		case protocol.ItemTypeQuestion:
			if it.Question != nil {
				fmt.Fprintf(&b, "## Question\n\n%s\n\n", it.Question.Prompt)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// contentText returns the text of an item (image blocks render as "[image]").
func contentText(it protocol.Item) string {
	if it.Text != "" {
		return it.Text
	}
	var parts []string
	for _, c := range it.Content {
		switch c.Type {
		case protocol.ContentBlockText:
			parts = append(parts, c.Text)
		case protocol.ContentBlockImage:
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}
