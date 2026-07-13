package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// wireSessionErr maps the session domain's not-found sentinel onto the wire
// sentinel, passing every other error through unchanged.
func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	if errors.Is(err, session.ErrTitleRequired) {
		return fmt.Errorf("%w: title must not be empty", protocol.ErrInvalidParams)
	}
	if errors.Is(err, session.ErrCwdUnavailable) {
		return fmt.Errorf("%w: %v", protocol.ErrCwdUnavailable, err)
	}
	return err
}

// defaultSessionPageLimit caps a single sessions.list page when the client
// gives no (or an over-large) limit.
const defaultSessionPageLimit = 100

// ListSessions paginates over the in-process session store with the
// same opaque-cursor mechanics as items.list (see pageByID): a non-empty
// NextCursor is the "has more" signal — never a silent truncation. The
// store returns the full ordered list; pagination is applied here.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	page, next := pageByID(sessions, func(ses session.Session) string { return ses.ID }, q.Cursor, q.Limit, defaultSessionPageLimit)
	running := s.runningSessionSet()
	waiting := s.waitingSessionSet(ctx)
	data := make([]protocol.Session, 0, len(page))
	for _, ses := range page {
		data = append(data, s.sessionToWire(ses, sessionStatus(running[ses.ID], waiting[ses.ID])))
	}
	return &protocol.Page[protocol.Session]{Data: data, NextCursor: next}, nil
}

func (s *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	ses, err := s.sessions.Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &out, nil
}

func (s *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// cwd defaults to the serve directory (ServerInfo.cwd) when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := in.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	ses, err := s.sessions.Create(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	// A freshly created session has no run and no interrupt — idle.
	out := s.sessionToWire(ses, protocol.SessionStatusIdle)
	return &out, nil
}

func (s *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	// The lifecycle coordinator claims the addressed session and every owned
	// internal-subtask descendant before deleting their durable state atomically.
	// User-created forks remain independent conversations.
	if err := s.sessions.DeleteSession(ctx, s.coordinator, id); err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return fmt.Errorf("%w: session %q or its subtask tree has a run in flight", protocol.ErrSessionBusy, id)
		}
		return wireSessionErr(err)
	}
	return nil
}

// UpdateSession applies a sessions.update edit: title (rename), model,
// cwd (relocate, gated by features.relocate) and metadata (full replace) are
// all live. Nil fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	ses, err := s.sessions.Update(ctx, s.coordinator, in.SessionID, session.Patch{
		Title:    in.Title,
		Model:    in.Model,
		Cwd:      in.Cwd,
		Metadata: in.Metadata,
		Favorite: in.Favorite,
	})
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, in.SessionID)
		}
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
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
	child, err := s.sessions.Fork(ctx, sessions.ForkSpec{
		ParentID:  in.SessionID,
		FromRunID: in.FromRunID,
		Title:     in.Title,
	})
	if err != nil {
		if in.FromRunID != "" {
			err = wireBoundaryErr(err)
		}
		return nil, wireSessionErr(err)
	}
	// A freshly forked child has no run of its own yet — idle.
	out := s.sessionToWire(child, protocol.SessionStatusIdle)
	return &out, nil
}

// sessionToWire converts the internal session shape into the wire shape.
// Status is supplied by the caller (see liveStatus / sessionStatus) so the
// list path can batch the lookups instead of querying per session. Model falls
// back to the runtime default when the session never explicitly selected one,
// so the wire always carries a real model name (the frontend resolves the
// assistant's displayName from it).
func (s *Server) sessionToWire(ses session.Session, status protocol.SessionStatus) protocol.Session {
	meta := ses.Metadata
	if meta == nil {
		meta = map[string]any{} // Session.metadata is an object, never null (API.md §4.1)
	}
	return protocol.Session{
		ID:        ses.ID,
		Title:     ses.Title,
		Cwd:       ses.Cwd,
		Model:     ses.EffectiveModel(s.models.DefaultModel()),
		Status:    status,
		CreatedAt: ses.StartedAt,
		UpdatedAt: ses.UpdatedAt,
		Favorite:  ses.Favorite,
		Metadata:  meta,
	}
}

// sessionStatus picks the wire status from the two live signals: running wins
// (an active run is the loudest state), then waiting (an open HITL interrupt),
// else idle.
func sessionStatus(running, waiting bool) protocol.SessionStatus {
	switch {
	case running:
		return protocol.SessionStatusRunning
	case waiting:
		return protocol.SessionStatusWaiting
	default:
		return protocol.SessionStatusIdle
	}
}

// liveStatus derives one session's status — running from the in-memory run
// registry, waiting from a targeted open-interrupt lookup. For the list path
// use the batched runningSessionSet / waitingSessionSet instead (this would be
// an N+1 there).
func (s *Server) liveStatus(ctx context.Context, sessionID string) protocol.SessionStatus {
	if s.hasActiveRun(sessionID) {
		return protocol.SessionStatusRunning
	}
	waiting := false
	if pending, err := s.queries.ListPendingInterrupts(ctx, sessionID); err == nil {
		waiting = len(pending) > 0
	}
	return sessionStatus(false, waiting)
}

// runningSessionSet snapshots the session ids with a live run, in one lock pass
// — the list path's batched form of hasActiveRun (rollback.go).
func (s *Server) runningSessionSet() map[string]bool {
	return s.coordinator.ActiveSessions()
}

// waitingSessionSet fetches every open interrupt once and returns the set of
// sessions awaiting a HITL answer — the list path's batched form, so per-session
// status costs no extra query. Empty on error (status degrades to running/idle).
func (s *Server) waitingSessionSet(ctx context.Context) map[string]bool {
	pending, err := s.queries.ListPendingInterrupts(ctx, "")
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(pending))
	for _, p := range pending {
		set[p.SessionID] = true
	}
	return set
}
