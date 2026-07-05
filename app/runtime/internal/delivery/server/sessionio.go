package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/core/model/chat"
)

// ExportSession serializes a session to a portable artifact (AUX_API §4.3).
// format=json (default) produces a round-trippable SessionArtifact —
// metadata + chat history + items + runs — that ImportSession restores
// verbatim. format=md produces a human-readable transcript (not re-importable).
// Returned inline: lyra is a local loopback runtime, so there's no out-of-band
// file channel nor a giant-payload concern.
func (s *Server) ExportSession(ctx context.Context, in protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	ses, err := s.rt.GetSession(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	items, runs, err := s.rt.ListTranscript(ctx, in.SessionID)
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
			RunID: r.RunID, UpdatedAt: r.UpdatedAt, MessageMark: r.Mark, Run: r.Blob,
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
			return nil, fmt.Errorf("%w: artifact.messages[%d]: %w", protocol.ErrInvalidParams, i, err)
		}
		msgs = append(msgs, m)
	}

	id := art.Session.ID
	admission, err := s.rt.ClaimRunSlot(ctx, sessionClaimer{s: s}, id)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, id)
		}
		return nil, err
	}
	defer admission.Release()

	// Map the wire artifact's runs/items into domain records (the wire→domain
	// decode is the adapter's job), then hand the restore to the lifecycle
	// coordinator. It commits the whole thing as ONE transaction — upsert the
	// session row, replace existing history (drop old items/runs + clear the
	// chat log + stale open interrupts), re-seed the messages, re-persist
	// runs+items — so a mid-sequence failure after the destructive
	// delete/truncate can't leave the session row live but its history
	// half-destroyed (an import-over losing the prior history with nothing to
	// replace it).
	runs := make([]transcript.Run, 0, len(art.Runs))
	for _, r := range art.Runs {
		runs = append(runs, transcript.Run{
			SessionID: id, RunID: r.RunID, UpdatedAt: r.UpdatedAt, Blob: r.Run, Mark: r.MessageMark,
		})
	}
	items := make([]transcript.Item, 0, len(art.Items))
	for _, it := range art.Items {
		items = append(items, transcript.Item{
			SessionID: id, RunID: it.RunID, ItemID: it.ItemID, CreatedAt: it.CreatedAt, Blob: it.Item,
		})
	}
	if err := s.rt.RestoreSession(ctx, artifactToSession(art.Session), msgs, runs, items); err != nil {
		return nil, err
	}

	ses, err := s.rt.GetSession(ctx, id)
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
