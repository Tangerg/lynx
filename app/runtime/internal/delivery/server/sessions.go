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
		return fmt.Errorf("%w: %w", protocol.ErrCwdUnavailable, err)
	}
	if errors.Is(err, session.ErrRevisionConflict) {
		return fmt.Errorf("%w: the session changed after it was read", protocol.ErrRevisionConflict)
	}
	return err
}

// defaultSessionPageLimit caps a single sessions.list page when the client
// gives no (or an over-large) limit.
const defaultSessionPageLimit = 100

// ListSessions paginates over the in-process session store with the
// same opaque-cursor mechanics as items.list (see pageByCursor): a non-empty
// NextCursor is the "has more" signal — never a silent truncation. The
// store returns the full ordered list; pagination is applied here.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	views, err := s.sessions.ListViews(ctx)
	if err != nil {
		return nil, err
	}
	page, next, err := pageByCursor(views, func(view sessions.SessionView) string { return view.ID }, q.Cursor, q.Limit, defaultSessionPageLimit)
	if err != nil {
		return nil, err
	}
	data := make([]protocol.Session, 0, len(page))
	for _, view := range page {
		data = append(data, sessionViewToWire(view))
	}
	return &protocol.Page[protocol.Session]{Data: data, NextCursor: next}, nil
}

func (s *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	view, err := s.sessions.View(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := sessionViewToWire(view)
	return &out, nil
}

func (s *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// cwd defaults to the serve directory (ServerInfo.cwd) when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := in.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	view, err := s.sessions.CreateView(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	out := sessionViewToWire(view)
	return &out, nil
}

func (s *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	// The lifecycle coordinator claims the addressed session and every owned
	// internal-subtask descendant before deleting their durable state atomically.
	// User-created forks remain independent conversations.
	if err := s.sessions.DeleteSession(ctx, id); err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return fmt.Errorf("%w: session %q or its subtask tree has a run in flight", protocol.ErrSessionBusy, id)
		}
		return wireSessionErr(err)
	}
	return nil
}

// UpdateSession applies a sessions.update edit: title (rename), model, cwd
// (relocate, gated by features.relocate), and favorite are all live. Nil
// fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	view, err := s.sessions.UpdateView(ctx, in.SessionID, session.Patch{
		Title:            in.Title,
		Model:            in.Model,
		Cwd:              in.Cwd,
		Favorite:         in.Favorite,
		ExpectedRevision: in.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, in.SessionID)
		}
		return nil, wireSessionErr(err)
	}
	out := sessionViewToWire(view)
	return &out, nil
}

// ForkSession branches a session into a fresh child that continues from the
// parent's conversation (API.md §7.2 / AUX_API §4.2): the child inherits the
// parent's cwd and a copy of its chat history, then diverges. An optional title
// overrides the default "<parent> (fork)".
//
// FromRunID (run-boundary fork — "branch from this run", B4) truncate-copies
// history up to and INCLUDING that run's turn; omit it for a whole-conversation
// fork. Snapshot semantics: only terminal runs are copied; an in-flight run and
// all of its mutable history tail are excluded. Forking deletes nothing, so
// unlike rollback it needs no session_busy guard.
func (s *Server) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	child, err := s.sessions.ForkView(ctx, sessions.ForkSpec{
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
	out := sessionViewToWire(child)
	return &out, nil
}

// sessionViewToWire projects the complete Application read model into the
// selected protocol shape. It intentionally performs no filesystem, live-run,
// or model-default lookup.
func sessionViewToWire(view sessions.SessionView) protocol.Session {
	return protocol.Session{
		ID:          view.ID,
		Title:       view.Title,
		Cwd:         view.Cwd,
		ProjectRoot: view.ProjectRoot,
		CwdMissing:  view.CwdMissing,
		Model:       view.Model,
		Status:      sessionStateToWire(view.State),
		CreatedAt:   view.CreatedAt,
		UpdatedAt:   view.UpdatedAt,
		Favorite:    view.Favorite,
		Revision:    view.Revision,
	}
}

// sessionStatus picks the wire status from the two live signals: running wins
// (an active run is the loudest state), then waiting (an open HITL interrupt),
// else idle.
func sessionStateToWire(state sessions.SessionState) protocol.SessionStatus {
	switch state {
	case sessions.SessionRunning:
		return protocol.SessionStatusRunning
	case sessions.SessionWaiting:
		return protocol.SessionStatusWaiting
	default:
		return protocol.SessionStatusIdle
	}
}
