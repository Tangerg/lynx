package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
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

// ListSessions paginates over the in-process session.Service. The
// backing store has no cursor pagination yet — the full list comes back
// as one page when no cursor is supplied; a cursor returns
// capability_not_negotiated until the store grows real pagination.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	if q.Cursor != "" {
		return nil, notImpl("sessions.list (cursor)")
	}
	sessions, err := s.rt.Session().List(ctx)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if limit > len(sessions) {
		limit = len(sessions)
	}
	data := make([]protocol.Session, 0, limit)
	for _, ses := range sessions[:limit] {
		data = append(data, s.sessionToWire(ses))
	}
	// No NextCursor: the store returns a single page (cursor paging is
	// gated off above). Emitting a cursor would point at an erroring call.
	return &protocol.Page[protocol.Session]{Data: data}, nil
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
	return nil
}

// UpdateSession applies a sessions.update edit. Title (rename) and model are
// live; cwd-relocate and metadata edits aren't backed yet and report
// capability_not_negotiated (features.relocate off). Nil fields are left alone;
// the updated session is returned. The dispatch layer already rejects an empty
// SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	if in.Cwd != nil {
		return nil, notImpl("sessions.update (relocate)")
	}
	if in.Metadata != nil {
		return nil, notImpl("sessions.update (metadata)")
	}

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

	ses, err := s.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses)
	return &out, nil
}

// ForkSession — fork at an item boundary depends on the checkpoint /
// item-id model, which isn't reconciled with the engine's history yet.
// Gated off (features.checkpoints).
func (s *Server) ForkSession(_ context.Context, _ protocol.ForkSessionRequest) (*protocol.Session, error) {
	return nil, notImpl("sessions.fork")
}

// ExportSession — needs a transport file channel to serve the artifact.
// Gated off (features.sessionExport).
func (s *Server) ExportSession(_ context.Context, _ protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	return nil, notImpl("sessions.export")
}

// sessionToWire converts the internal session shape into the wire shape.
// Status is synthesized (internal sessions don't track running/waiting/
// idle yet → idle). Metadata widens map[string]string → map[string]any.
// Model falls back to the runtime default when the session never explicitly
// selected one, so the wire always carries a real model name (the frontend
// resolves the assistant's displayName from it).
func (s *Server) sessionToWire(ses session.Session) protocol.Session {
	var meta map[string]any
	if len(ses.Metadata) > 0 {
		meta = make(map[string]any, len(ses.Metadata))
		for k, v := range ses.Metadata {
			meta[k] = v
		}
	}
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
