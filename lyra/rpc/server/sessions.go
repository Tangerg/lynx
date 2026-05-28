package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListSessions paginates over the in-process session.Service.
// The current backing store doesn't expose cursor pagination yet —
// we treat the full list as one page when no cursor is supplied,
// and return ErrNotImplemented when a cursor is passed (forces an
// honest signal to the client until the store grows real pagination).
func (i *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	if q.Cursor != "" {
		return nil, protocol.ErrNotImplemented
	}
	sessions, err := i.rt.Session().List(ctx)
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
	items := make([]protocol.Session, 0, limit)
	for _, s := range sessions[:limit] {
		items = append(items, sessionToWire(s))
	}
	return &protocol.Page[protocol.Session]{
		Items:   items,
		HasMore: len(sessions) > limit,
	}, nil
}

func (i *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	s, err := i.rt.Session().Get(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, protocol.ErrSessionNotFound
		}
		return nil, err
	}
	out := sessionToWire(s)
	return &out, nil
}

func (i *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	s, err := i.rt.Session().Create(ctx, in.Title)
	if err != nil {
		return nil, err
	}
	out := sessionToWire(s)
	return &out, nil
}

// UpdateSession — session.Service has no update verb yet. Stub.
func (i *Server) UpdateSession(_ context.Context, _ protocol.UpdateSessionRequest) (*protocol.Session, error) {
	return nil, notImpl("sessions.update")
}

func (i *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("sessions.delete: id is required")
	}
	if err := i.rt.Session().Delete(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return protocol.ErrSessionNotFound
		}
		return err
	}
	return nil
}

func (i *Server) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	if in.ParentID == "" || in.AtMessageID == "" {
		return nil, errors.New("sessions.fork: parentId + atMessageId required")
	}
	s, err := i.rt.Session().Fork(ctx, in.ParentID, in.AtMessageID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, protocol.ErrSessionNotFound
		}
		return nil, err
	}
	out := sessionToWire(s)
	return &out, nil
}

// ExportSession — no file-serving endpoint backed yet. Once a
// downloadable artifact endpoint exists, return its URL here.
func (i *Server) ExportSession(_ context.Context, _ protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	return nil, notImpl("sessions.export")
}

// sessionToWire converts the internal session shape into the wire
// shape. Status is synthesized — internal Sessions don't track an
// explicit "running/waiting/idle" flag yet, so we default to idle.
// Metadata widens map[string]string → map[string]any at the boundary;
// internal store stays string-only.
func sessionToWire(s session.Session) protocol.Session {
	var meta map[string]any
	if len(s.Metadata) > 0 {
		meta = make(map[string]any, len(s.Metadata))
		for k, v := range s.Metadata {
			meta[k] = v
		}
	}
	return protocol.Session{
		ID:        s.ID,
		Title:     s.Title,
		Status:    protocol.SessionStatusIdle,
		Model:     "", // future: snapshot the model used for the last turn
		CreatedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
		Metadata:  meta,
	}
}
