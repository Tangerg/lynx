package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/service/transcript"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// wireSessionErr maps the session domain's not-found sentinel onto the wire
// sentinel, passing every other error through unchanged.
func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	return err
}

// defaultSessionPageLimit caps a single sessions.list page when the client
// gives no (or an over-large) limit.
const defaultSessionPageLimit = 100

// ListSessions paginates over the in-process session.Service with the
// same opaque-cursor mechanics as items.list (see pageByID): a non-empty
// NextCursor is the "has more" signal — never a silent truncation. The
// store returns the full ordered list; pagination is applied here.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	sessions, err := s.rt.Session().List(ctx)
	if err != nil {
		return nil, err
	}
	page, next := pageByID(sessions, func(ses session.Session) string { return ses.ID }, q.Cursor, q.Limit, defaultSessionPageLimit)
	data := make([]protocol.Session, 0, len(page))
	for _, ses := range page {
		data = append(data, s.sessionToWire(ses))
	}
	return &protocol.Page[protocol.Session]{Data: data, NextCursor: next}, nil
}

func (s *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	ses, err := s.rt.Session().Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses)
	return &out, nil
}

func (s *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// cwd defaults to the serve directory (ServerInfo.cwd) when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := in.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	ses, err := s.rt.Session().Create(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	out := s.sessionToWire(ses)
	return &out, nil
}

func (s *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	if err := s.rt.Session().Delete(ctx, id); err != nil {
		return wireSessionErr(err)
	}
	s.dropCheckpoints(id) // best-effort: discard the session's file snapshots
	return nil
}

// UpdateSession applies a sessions.update edit: title (rename), model,
// cwd (relocate, gated by features.relocate) and metadata (full replace) are
// all live. Nil fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return nil, fmt.Errorf("%w: title must not be empty", protocol.ErrInvalidParams)
		}
		if err := s.rt.Session().Rename(ctx, in.SessionID, title); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Model != nil {
		if err := s.rt.Session().SetModel(ctx, in.SessionID, *in.Model); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Cwd != nil {
		// Relocate to a real, existing directory — a stale path would silently
		// break every later run's tool/memory resolution, so reject it now.
		info, err := os.Stat(*in.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: %s", protocol.ErrCwdUnavailable, *in.Cwd)
		}
		if err := s.rt.Session().SetCwd(ctx, in.SessionID, *in.Cwd); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Metadata != nil {
		if err := s.rt.Session().SetMetadata(ctx, in.SessionID, *in.Metadata); err != nil {
			return nil, wireSessionErr(err)
		}
	}

	ses, err := s.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses)
	return &out, nil
}

// ForkSession branches a session into a fresh child that continues from the
// parent's conversation (API.md §7.2 / AUX_API §4.2): the child inherits the
// parent's cwd and a copy of its chat history, then diverges. An optional title
// overrides the default "<parent> (fork)".
//
// FromRunID (run-boundary fork — "branch from this run", B4) truncate-copies
// history up to and INCLUDING that run's turn; omit it for a whole-conversation
// fork. Snapshot semantics: only completed runs are copied, so an in-flight run
// at the boundary contributes only what it has already flushed. Forking deletes
// nothing, so unlike rollback it needs no session_busy guard.
func (s *Server) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	// Resolve the copy boundary against the parent BEFORE creating the child.
	// copyN < 0 means "copy the whole history".
	copyN := -1
	if in.FromRunID != "" {
		_, runs, err := s.rt.Transcript().List(ctx, in.SessionID)
		if err != nil {
			return nil, wireSessionErr(err)
		}
		nodes, _, err := runNodes(runs)
		if err != nil {
			return nil, err
		}
		// requireRoot=false: fork is lax about the boundary run's kind (the
		// contract lists only session_not_found / run_not_found).
		b, err := transcript.BoundaryAt(nodes, in.FromRunID, false)
		if err != nil {
			return nil, wireBoundaryErr(err)
		}
		copyN = b.KeepMark // -1 (unknown watermark) falls back to a full copy below
	}

	// Fork records the branch lineage (parent id, inherited cwd); atMessageID
	// is empty because lineage is run-based now, not message-based.
	child, err := s.rt.Session().Fork(ctx, in.SessionID, "")
	if err != nil {
		return nil, wireSessionErr(err)
	}

	// Copy the parent's history prefix into the fresh child so its next turn
	// continues with the same context. The child was just created (empty), so
	// the append-only seed can't double up.
	msgs, err := s.rt.ReadHistory(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	if copyN >= 0 && copyN < len(msgs) {
		msgs = msgs[:copyN]
	}
	if err := s.rt.SeedHistory(ctx, child.ID, msgs); err != nil {
		return nil, err
	}

	if in.Title != "" {
		if err := s.rt.Session().Rename(ctx, child.ID, in.Title); err != nil {
			return nil, wireSessionErr(err)
		}
		child.Title = in.Title
	}

	out := s.sessionToWire(child)
	return &out, nil
}

// sessionToWire converts the internal session shape into the wire shape.
// Status is synthesized (internal sessions don't track running/waiting/
// idle yet → idle). Model falls back to the runtime default when the session
// never explicitly selected one, so the wire always carries a real model name
// (the frontend resolves the assistant's displayName from it).
func (s *Server) sessionToWire(ses session.Session) protocol.Session {
	meta := ses.Metadata
	if meta == nil {
		meta = map[string]any{} // Session.metadata is an object, never null (API.md §4.1)
	}
	return protocol.Session{
		ID:        ses.ID,
		Title:     ses.Title,
		Cwd:       ses.Cwd,
		Model:     ses.EffectiveModel(s.rt.DefaultModel()),
		Status:    protocol.SessionStatusIdle,
		CreatedAt: ses.StartedAt,
		UpdatedAt: ses.UpdatedAt,
		Metadata:  meta,
	}
}
