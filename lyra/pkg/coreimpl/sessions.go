package coreimpl

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// ListSessions paginates over the in-process session.Service.
// The current backing store doesn't expose cursor pagination yet —
// we treat the full list as one page when no cursor is supplied,
// and return ErrNotImplemented when a cursor is passed (forces an
// honest signal to the client until the store grows real pagination).
func (i *Impl) ListSessions(ctx context.Context, q coreapi.PageQuery) (*coreapi.Page[coreapi.Session], error) {
	if q.Cursor != "" {
		return nil, coreapi.ErrNotImplemented
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
	items := make([]coreapi.Session, 0, limit)
	for _, s := range sessions[:limit] {
		items = append(items, sessionToCoreAPI(s))
	}
	return &coreapi.Page[coreapi.Session]{
		Items:   items,
		HasMore: len(sessions) > limit,
	}, nil
}

func (i *Impl) GetSession(ctx context.Context, id string) (*coreapi.Session, error) {
	s, err := i.rt.Session().Get(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, coreapi.ErrSessionNotFound
		}
		return nil, err
	}
	out := sessionToCoreAPI(s)
	return &out, nil
}

func (i *Impl) CreateSession(ctx context.Context, in coreapi.CreateSessionIn) (*coreapi.Session, error) {
	s, err := i.rt.Session().Create(ctx, in.Title)
	if err != nil {
		return nil, err
	}
	out := sessionToCoreAPI(s)
	return &out, nil
}

// UpdateSession — session.Service has no update verb yet. Stub.
func (i *Impl) UpdateSession(_ context.Context, _ coreapi.UpdateSessionIn) (*coreapi.Session, error) {
	return nil, notImpl("sessions.update")
}

func (i *Impl) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("sessions.delete: id is required")
	}
	if err := i.rt.Session().Delete(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return coreapi.ErrSessionNotFound
		}
		return err
	}
	return nil
}

func (i *Impl) ForkSession(ctx context.Context, in coreapi.ForkSessionIn) (*coreapi.Session, error) {
	if in.ParentID == "" || in.AtMessageID == "" {
		return nil, errors.New("sessions.fork: parentId + atMessageId required")
	}
	s, err := i.rt.Session().Fork(ctx, in.ParentID, in.AtMessageID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, coreapi.ErrSessionNotFound
		}
		return nil, err
	}
	out := sessionToCoreAPI(s)
	return &out, nil
}

// ExportSession — no file-serving endpoint backed yet. Once a
// downloadable artefact endpoint exists, return its URL here.
func (i *Impl) ExportSession(_ context.Context, _ coreapi.ExportSessionIn) (*coreapi.ExportSessionOut, error) {
	return nil, notImpl("sessions.export")
}

// sessionToCoreAPI converts the internal session shape into the wire
// shape. Status is synthesised — internal Sessions don't track an
// explicit "running/waiting/idle" flag yet, so we default to idle.
// Metadata widens map[string]string → map[string]any at the boundary;
// internal store stays string-only.
func sessionToCoreAPI(s session.Session) coreapi.Session {
	var meta map[string]any
	if len(s.Metadata) > 0 {
		meta = make(map[string]any, len(s.Metadata))
		for k, v := range s.Metadata {
			meta[k] = v
		}
	}
	return coreapi.Session{
		ID:        s.ID,
		Title:     s.Title,
		Status:    coreapi.SessionStatusIdle,
		Model:     "", // future: snapshot the model used for the last turn
		CreatedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
		Metadata:  meta,
	}
}
