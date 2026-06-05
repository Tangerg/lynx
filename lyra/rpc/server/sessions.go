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
func (i *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	if q.Cursor != "" {
		return nil, notImpl("sessions.list (cursor)")
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
	data := make([]protocol.Session, 0, limit)
	for _, s := range sessions[:limit] {
		data = append(data, i.sessionToWire(s))
	}
	// No NextCursor: the store returns a single page (cursor paging is
	// gated off above). Emitting a cursor would point at an erroring call.
	return &protocol.Page[protocol.Session]{Data: data}, nil
}

func (i *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	s, err := i.rt.Session().Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := i.sessionToWire(s)
	return &out, nil
}

func (i *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// cwd defaults to the serve directory (ServerInfo.cwd) when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := in.Cwd
	if cwd == "" {
		cwd = i.serverInfo.Cwd
	}
	s, err := i.rt.Session().Create(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	out := i.sessionToWire(s)
	return &out, nil
}

func (i *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	if err := i.rt.Session().Delete(ctx, id); err != nil {
		return wireSessionErr(err)
	}
	return nil
}

// UpdateSession applies a sessions.update edit. Title (rename) and model are
// live; cwd-relocate and metadata edits aren't backed yet and report
// capability_not_negotiated (features.relocate off). Nil fields are left alone;
// the updated session is returned. The dispatch layer already rejects an empty
// SessionID.
func (i *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
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
		if err := i.rt.Session().Rename(ctx, in.SessionID, title); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Model != nil {
		if err := i.rt.Session().SetModel(ctx, in.SessionID, *in.Model); err != nil {
			return nil, wireSessionErr(err)
		}
	}

	s, err := i.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := i.sessionToWire(s)
	return &out, nil
}

// ForkSession — fork at an item boundary depends on the checkpoint /
// item-id model, which isn't reconciled with the engine's history yet.
// Gated off (features.checkpoints).
func (i *Server) ForkSession(_ context.Context, _ protocol.ForkSessionRequest) (*protocol.Session, error) {
	return nil, notImpl("sessions.fork")
}

// ExportSession — needs a transport file channel to serve the artifact.
// Gated off (features.sessionExport).
func (i *Server) ExportSession(_ context.Context, _ protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	return nil, notImpl("sessions.export")
}

// sessionToWire converts the internal session shape into the wire shape.
// Status is synthesized (internal sessions don't track running/waiting/
// idle yet → idle). Metadata widens map[string]string → map[string]any.
// Model falls back to the runtime default when the session never explicitly
// selected one, so the wire always carries a real model name (the frontend
// resolves the assistant's displayName from it).
func (i *Server) sessionToWire(s session.Session) protocol.Session {
	var meta map[string]any
	if len(s.Metadata) > 0 {
		meta = make(map[string]any, len(s.Metadata))
		for k, v := range s.Metadata {
			meta[k] = v
		}
	}
	if meta == nil {
		meta = map[string]any{} // Session.metadata is an object, never null (API.md §4.1)
	}
	return protocol.Session{
		ID:        s.ID,
		Title:     s.Title,
		Cwd:       s.Cwd,
		Model:     s.EffectiveModel(i.rt.DefaultModel()),
		Status:    protocol.SessionStatusIdle,
		CreatedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
		Metadata:  meta,
	}
}
