package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type sessionCatalogBinding interface {
	ListSessions(context.Context, protocol.PageQuery, embedded.CallOptions) (*protocol.Page[protocol.Session], error)
	CreateSession(context.Context, protocol.CreateSessionRequest, embedded.CommandOptions) (*protocol.Session, error)
	UpdateSession(context.Context, protocol.UpdateSessionRequest, embedded.CommandOptions) (*protocol.Session, error)
	ForkSession(context.Context, protocol.ForkSessionRequest, embedded.CommandOptions) (*protocol.Session, error)
	DeleteSession(context.Context, protocol.DeleteSessionRequest, embedded.CommandOptions) error
}

const (
	defaultSessionPageSize = 20
	maximumSessionPageSize = 100
)

func (r *Runtime) ListSessions(ctx context.Context, query agent.SessionQuery) (agent.SessionPage, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = defaultSessionPageSize
	}
	if limit > maximumSessionPageSize {
		limit = maximumSessionPageSize
	}

	search := strings.ToLower(strings.TrimSpace(query.Search))
	workspace := strings.TrimSpace(query.Workspace)
	if search == "" && workspace == "" {
		page, err := r.sessionCatalog.ListSessions(ctx, protocol.PageQuery{Limit: limit, Cursor: query.Cursor}, r.callOptions())
		if err != nil {
			return agent.SessionPage{}, classifyError(err)
		}
		return projectSessionPage(page)
	}

	// Search and workspace are CLI projections absent from the runtime protocol.
	// Walk opaque cursors one row at a time so NextCursor remains the exact
	// continuation after the last examined runtime row, including filtered rows.
	result := agent.SessionPage{Items: make([]agent.Session, 0, limit)}
	cursors := newCursorTraversal("list sessions", query.Cursor)
	for len(result.Items) < limit {
		cursor := cursors.Current()
		page, err := r.sessionCatalog.ListSessions(ctx, protocol.PageQuery{Limit: 1, Cursor: cursor}, r.callOptions())
		if err != nil {
			return agent.SessionPage{}, classifyError(err)
		}
		if page == nil {
			return agent.SessionPage{}, errors.New("list sessions: runtime returned a nil page")
		}
		if len(page.Data) > 1 {
			return agent.SessionPage{}, errors.New("list sessions: runtime exceeded the requested one-row page")
		}
		for _, value := range page.Data {
			projected, err := projectSession(value)
			if err != nil {
				return agent.SessionPage{}, err
			}
			if matchesSession(projected, search, workspace) {
				result.Items = append(result.Items, projected)
			}
		}
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return agent.SessionPage{}, err
		}
		if !more {
			break
		}
	}
	result.NextCursor = cursors.Current()
	if err := result.Validate(); err != nil {
		return agent.SessionPage{}, fmt.Errorf("list sessions projection: %w", err)
	}
	return result, nil
}

func matchesSession(session agent.Session, search, workspace string) bool {
	if workspace != "" && session.Workspace != workspace {
		return false
	}
	return search == "" || strings.Contains(strings.ToLower(session.Title), search) ||
		strings.Contains(strings.ToLower(session.Workspace), search)
}

func projectSessionPage(page *protocol.Page[protocol.Session]) (agent.SessionPage, error) {
	if page == nil {
		return agent.SessionPage{}, errors.New("list sessions: runtime returned a nil page")
	}
	result := agent.SessionPage{Items: make([]agent.Session, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		projected, err := projectSession(value)
		if err != nil {
			return agent.SessionPage{}, err
		}
		result.Items = append(result.Items, projected)
	}
	if err := result.Validate(); err != nil {
		return agent.SessionPage{}, fmt.Errorf("list sessions projection: %w", err)
	}
	return result, nil
}

func projectSession(value protocol.Session) (agent.Session, error) {
	status := agent.SessionStatus(value.Status)
	projected := agent.Session{
		ID: value.ID, Title: value.Title, Status: status, Model: value.Model,
		Workspace: value.Workspace.Ref.Path, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Favorite: value.Favorite, Revision: value.Revision,
	}
	if err := projected.Validate(); err != nil {
		return agent.Session{}, fmt.Errorf("runtime session %q: %w", value.ID, err)
	}
	return projected, nil
}

func (r *Runtime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	request := protocol.CreateSessionRequest{Title: input.Title}
	if input.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: input.Workspace}
	}
	created, err := r.sessionCatalog.CreateSession(ctx, request, options)
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if created == nil {
		return agent.Session{}, errors.New("create session: runtime returned nil")
	}
	return projectSession(*created)
}

func (r *Runtime) UpdateSession(ctx context.Context, input agent.UpdateSession) (agent.Session, error) {
	if err := input.Validate(); err != nil {
		return agent.Session{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	request := protocol.UpdateSessionRequest{
		SessionID: input.SessionID, ExpectedRevision: input.ExpectedRevision,
		Title: input.Title, Model: input.Model, Favorite: input.Favorite,
	}
	if input.Workspace != nil {
		request.Workspace = &protocol.WorkspaceRef{Path: *input.Workspace}
	}
	updated, err := r.sessionCatalog.UpdateSession(ctx, request, options)
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if updated == nil {
		return agent.Session{}, errors.New("update session: runtime returned nil")
	}
	return projectSession(*updated)
}

func (r *Runtime) ForkSession(ctx context.Context, input agent.ForkSession) (agent.Session, error) {
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	forked, err := r.sessionCatalog.ForkSession(ctx, protocol.ForkSessionRequest{
		SessionID: input.SessionID, FromRunID: input.FromRunID, Title: input.Title,
	}, options)
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if forked == nil {
		return agent.Session{}, errors.New("fork session: runtime returned nil")
	}
	return projectSession(*forked)
}

func (r *Runtime) DeleteSession(ctx context.Context, input agent.DeleteSession) error {
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.sessionCatalog.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: input.SessionID}, options))
}
