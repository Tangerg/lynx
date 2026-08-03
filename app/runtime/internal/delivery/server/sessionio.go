package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ExportSession serializes a session to a portable artifact (AUX_API §4.3).
// format=json (default) produces a round-trippable SessionArtifact —
// session identity + chat history + canonical items + runs + portable offloaded tool
// bodies — that ImportSession restores. format=md produces a human-readable
// transcript (not re-importable).
// Returned inline: lyra is a local loopback runtime, so there's no out-of-band
// file channel nor a giant-payload concern.
func (s *Server) ExportSession(ctx context.Context, in protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	result, err := s.sessions.ExportSession(ctx, in.SessionID)
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
	presentedSession := sessionViewToWire(result.Session)

	switch format {
	case protocol.ExportFormatMarkdown:
		return &protocol.ExportSessionResponse{
			Format:   format,
			Markdown: renderSessionMarkdown(presentedSession, result.Items),
		}, nil
	case protocol.ExportFormatJSON:
	default:
		return nil, fmt.Errorf("%w: unsupported export format %q", protocol.ErrInvalidParams, format)
	}
	artifact, err := artifactFromPortable(result.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("sessions.export: encode artifact: %w", err)
	}

	return &protocol.ExportSessionResponse{
		Format: format, Artifact: &artifact,
	}, nil
}

// ImportSession recreates a session from a SessionArtifact under its ORIGINAL
// id (restore semantics): it upserts the session record, replaces any existing
// history, then re-seeds the chat messages, canonical items/runs, and offloaded
// tool bodies. Re-importing the same artifact is idempotent; importing over an
// existing session restores it.
// refuseUnadvertisedStates rejects an archive carrying a state key this build does
// not own. It reads the same advertised set discovery publishes, so "the runtime
// offers this key" and "the runtime can import this key" cannot come apart.
func (s *Server) refuseUnadvertisedStates(states []protocol.ArtifactState) error {
	if len(states) == 0 {
		return nil
	}
	advertised := advertisedStateSnapshots(s.features.plan)
	var gaps []protocol.CapabilityRequirement
	for _, state := range states {
		key := protocol.StateSnapshotType(state.Type)
		if slices.ContainsFunc(advertised, func(c protocol.StateSnapshotCapability) bool { return c.Key == key }) {
			continue
		}
		gaps = append(gaps, protocol.CapabilityRequirement{
			Type: protocol.RequirementStateSnapshot, Name: string(state.Type),
		})
	}
	if len(gaps) == 0 {
		return nil
	}
	return protocol.NewCapabilityGap(gaps...)
}

func (s *Server) ImportSession(ctx context.Context, in protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
	art := in.Artifact
	if art.Version != protocol.SessionArtifactVersion {
		return nil, fmt.Errorf("%w: unsupported artifact version %d (want %d)", protocol.ErrInvalidParams, art.Version, protocol.SessionArtifactVersion)
	}
	portable, err := portableArtifactFromWire(art)
	if err != nil {
		return nil, err
	}
	// Before any write: an archive whose state keys this composition does not
	// advertise cannot be restored, and restoring the conversation while dropping
	// the key would import a session the archive does not describe. The gap names
	// the key so the caller learns WHICH one rather than that "something" is off.
	if err := s.refuseUnadvertisedStates(art.States); err != nil {
		return nil, err
	}
	id := art.Session.ID

	// Hand the strictly decoded portable archive to the lifecycle coordinator.
	// It commits the whole thing as ONE transaction — upsert the
	// session row, replace existing history (drop old items/runs/tool bodies + clear the
	// chat log + stale open interrupts), re-seed the messages, re-persist
	// runs+items+tool bodies — so a mid-sequence failure after the destructive
	// delete/truncate can't leave the session row live but its history
	// half-destroyed (an import-over losing the prior history with nothing to
	// replace it).
	view, err := s.sessions.RestorePortableSession(ctx, portable)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, id)
		}
		if errors.Is(err, sessions.ErrInvalidPortableSnapshot) {
			return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
		}
		if errors.Is(err, transcript.ErrIdentityConflict) || errors.Is(err, offload.ErrIdentityConflict) {
			return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
		}
		return nil, wireSessionErr(err)
	}

	out := sessionViewToWire(view)
	return &protocol.ImportSessionResponse{Session: &out}, nil
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
	if ses.Workspace.Ref.Path != "" {
		fmt.Fprintf(&b, "- workspace: `%s`\n", ses.Workspace.Ref.Path)
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
		case protocol.ItemTypeQuestion:
			if it.Question != nil {
				b.WriteString("## Question\n\n")
				for _, field := range it.Question.Fields {
					fmt.Fprintf(&b, "- %s\n", field.Prompt)
				}
				b.WriteString("\n")
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
