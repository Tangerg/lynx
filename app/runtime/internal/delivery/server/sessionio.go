package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ExportSession serializes a session to a portable artifact (AUX_API §4.3).
// format=json (default) produces a round-trippable SessionArtifact —
// metadata + chat history + items + runs — that ImportSession restores
// verbatim. format=md produces a human-readable transcript (not re-importable).
// Returned inline: lyra is a local loopback runtime, so there's no out-of-band
// file channel nor a giant-payload concern.
func (s *Server) ExportSession(ctx context.Context, in protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	ses, err := s.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	items, runs, err := s.rt.Transcript().List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}

	format := in.Format
	if format == "" {
		format = protocol.ExportFormatJSON
	}

	if format == protocol.ExportFormatMarkdown {
		return &protocol.ExportSessionResponse{
			Format:   format,
			Markdown: renderSessionMarkdown(s.sessionToWire(ses, s.liveStatus(ctx, ses.ID)), items),
		}, nil
	}

	msgs, err := s.rt.ReadHistory(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	msgBlobs := make([]json.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("sessions.export: marshal message: %w", err)
		}
		msgBlobs = append(msgBlobs, b)
	}

	artRuns := make([]protocol.ArtifactRun, 0, len(runs))
	for _, r := range runs {
		artRuns = append(artRuns, protocol.ArtifactRun{
			RunID: r.RunID, UpdatedAt: r.UpdatedAt, Mark: r.Mark, Run: r.Blob,
		})
	}
	artItems := make([]protocol.ArtifactItem, 0, len(items))
	for _, it := range items {
		artItems = append(artItems, protocol.ArtifactItem{
			RunID: it.RunID, ItemID: it.ItemID, CreatedAt: it.CreatedAt, Item: it.Blob,
		})
	}

	return &protocol.ExportSessionResponse{
		Format: format,
		Artifact: &protocol.SessionArtifact{
			Version:  protocol.SessionArtifactVersion,
			Session:  s.sessionToWire(ses, s.liveStatus(ctx, ses.ID)),
			Messages: msgBlobs,
			Runs:     artRuns,
			Items:    artItems,
		},
	}, nil
}

// ImportSession recreates a session from a SessionArtifact under its ORIGINAL
// id (restore semantics): it upserts the session record, replaces any existing
// history, then re-seeds the chat messages and re-persists the items + runs
// verbatim. Re-importing the same artifact is idempotent; importing over an
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
		m, err := chat.UnmarshalMessage(blob)
		if err != nil {
			return nil, fmt.Errorf("%w: artifact.messages[%d]: %v", protocol.ErrInvalidParams, i, err)
		}
		msgs = append(msgs, m)
	}

	id := art.Session.ID
	if err := s.rt.Session().Restore(ctx, artifactToSession(art.Session)); err != nil {
		return nil, err
	}

	// Replace any existing history so an import-over restores rather than
	// appends: drop the old items/runs and clear the chat log before re-seeding.
	if err := s.rt.Transcript().DeleteSession(ctx, id); err != nil {
		return nil, err
	}
	if err := s.rt.TruncateMessages(ctx, id, 0); err != nil {
		return nil, err
	}
	if err := s.rt.SeedHistory(ctx, id, msgs); err != nil {
		return nil, err
	}
	for _, r := range art.Runs {
		if err := s.rt.Transcript().PutRun(ctx, transcript.Run{
			SessionID: id, RunID: r.RunID, UpdatedAt: r.UpdatedAt, Blob: r.Run, Mark: r.Mark,
		}); err != nil {
			return nil, err
		}
	}
	for _, it := range art.Items {
		if err := s.rt.Transcript().AppendItem(ctx, transcript.Item{
			SessionID: id, RunID: it.RunID, ItemID: it.ItemID, CreatedAt: it.CreatedAt, Blob: it.Item,
		}); err != nil {
			return nil, err
		}
	}

	ses, err := s.rt.Session().Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &protocol.ImportSessionResponse{Session: &out}, nil
}

// artifactToSession maps the wire Session carried in an artifact back to the
// domain session for a verbatim restore. The wire shape omits the
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
// header plus each item rendered by type. Best-effort: an item whose blob
// can't be decoded is skipped. Not re-importable (use format=json for that).
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
		var it protocol.Item
		if err := json.Unmarshal(raw.Blob, &it); err != nil {
			continue
		}
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
		case "text":
			parts = append(parts, c.Text)
		case "image":
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}
