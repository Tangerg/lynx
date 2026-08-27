package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/application/sessions"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ExportSession serializes a session to a portable artifact (AUX_API §4.3).
// format=json (default) produces a round-trippable SessionArtifact —
// session identity + chat history + canonical items + runs + portable offloaded tool
// bodies — that ImportSession restores. format=md produces a human-readable
// transcript (not re-importable).
// Returned inline: scopeapp is a local loopback runtime, so there's no out-of-band
// file channel nor a giant-payload concern.
func (s *Server) ExportSession(ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	result, err := s.sessions.ExportSession(ctx, request.SessionID)
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, request.SessionID)
		}
		return nil, wireSessionErr(err)
	}

	format := request.Format
	if format == "" {
		format = protocol.ExportFormatJSON
	}
	presentedSession := presentSession(result.Session)

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

// validateArtifactPlanCapability rejects a Plan when this composition does not own
// Plan. Import must not restore the conversation while silently dropping companion
// product material.
func (s *Server) validateArtifactPlanCapability(plan []protocol.PlanStep) error {
	if len(plan) == 0 || s.features.plan {
		return nil
	}
	return operation.NewCapabilityGapError(protocol.CapabilityRequirement{
		Type: protocol.RequirementFeature, Name: protocol.FeaturePlan,
	})
}

// ImportSession recreates a Session from a SessionArtifact under its original
// identity. Re-importing the same artifact is idempotent; importing over an
// existing Session restores it atomically.
func (s *Server) ImportSession(ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
	artifact := request.Artifact
	if artifact.Version != protocol.SessionArtifactVersion {
		return nil, fmt.Errorf("%w: unsupported artifact version %d (want %d)", protocol.ErrInvalidParams, artifact.Version, protocol.SessionArtifactVersion)
	}
	portable, err := portableArtifactFromWire(artifact)
	if err != nil {
		return nil, err
	}
	// Before any write: restoring the conversation while dropping its Plan would
	// import a session the archive does not describe.
	if validateArtifactPlanCapabilityErr := s.validateArtifactPlanCapability(artifact.Plan); validateArtifactPlanCapabilityErr != nil {
		return nil, validateArtifactPlanCapabilityErr
	}
	sessionID := artifact.Session.ID

	// Hand the strictly decoded portable archive to the session use case.
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
			return nil, fmt.Errorf("%w: session %q has a run in flight or open interrupt", protocol.ErrSessionBusy, sessionID)
		}
		if errors.Is(err, sessions.ErrInvalidPortableSnapshot) {
			return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
		}
		if errors.Is(err, transcript.ErrIdentityConflict) || errors.Is(err, toolresult.ErrIdentityConflict) {
			return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
		}
		return nil, wireSessionErr(err)
	}

	presented := presentSession(view)
	return &protocol.ImportSessionResponse{Session: &presented}, nil
}

// renderSessionMarkdown produces a human-readable transcript of a session — a
// header plus each canonical item rendered by type. It is not re-importable
// (use format=json for that).
func renderSessionMarkdown(session protocol.Session, items []transcript.Item) string {
	var b strings.Builder
	title := session.Title
	if title == "" {
		title = session.ID
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if session.Workspace.Ref.Path != "" {
		fmt.Fprintf(&b, "- workspace: `%s`\n", session.Workspace.Ref.Path)
	}
	if session.Model != "" {
		fmt.Fprintf(&b, "- model: `%s`\n", session.Model)
	}
	b.WriteString("\n")

	for _, record := range items {
		item := presentItem(record)
		switch item.Type {
		case protocol.ItemTypeUserMessage:
			fmt.Fprintf(&b, "## User\n\n%s\n\n", contentText(item))
		case protocol.ItemTypeAgentMessage:
			fmt.Fprintf(&b, "## Assistant\n\n%s\n\n", contentText(item))
		case protocol.ItemTypeReasoning:
			if !item.Redacted && item.Text != "" {
				fmt.Fprintf(&b, "> _(reasoning)_ %s\n\n", item.Text)
			}
		case protocol.ItemTypeToolCall:
			if item.Tool != nil {
				fmt.Fprintf(&b, "→ **tool** `%s`\n\n", item.Tool.Name)
			}
		case protocol.ItemTypeQuestion:
			if item.Question != nil {
				b.WriteString("## Question\n\n")
				for _, field := range item.Question.Fields {
					fmt.Fprintf(&b, "- %s\n", field.Prompt)
				}
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// contentText returns the text of an item (image blocks render as "[image]").
func contentText(item protocol.Item) string {
	if item.Text != "" {
		return item.Text
	}
	var parts []string
	for _, block := range item.Content {
		switch block.Type {
		case protocol.ContentBlockText:
			parts = append(parts, block.Text)
		case protocol.ContentBlockImage:
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}
