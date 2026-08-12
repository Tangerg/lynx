package runtimeembedded

import (
	"context"
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
	query, err := query.Normalize()
	if err != nil {
		return agent.SessionPage{}, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultSessionPageSize
	}
	if limit > maximumSessionPageSize {
		limit = maximumSessionPageSize
	}

	search := strings.ToLower(query.Search)
	workspace := query.Workspace
	if search == "" && workspace == "" {
		page, err := r.sessionCatalog.ListSessions(ctx, protocol.PageQuery{Limit: limit, Cursor: query.Cursor}, r.callOptions())
		if err != nil {
			return agent.SessionPage{}, classifyError(err)
		}
		return projectSessionPage(page, query.Cursor, limit)
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
			return agent.SessionPage{}, runtimeContractViolation("list sessions returned a nil page")
		}
		if len(page.Data) > 1 {
			return agent.SessionPage{}, runtimeContractViolation("list sessions exceeded the requested one-row page")
		}
		for _, value := range page.Data {
			projected, err := projectSession(value)
			if err != nil {
				return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid session: %v", err)
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
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid projection: %v", err)
	}
	return result, nil
}

func matchesSession(session agent.Session, search, workspace string) bool {
	if workspace != "" && session.Workspace.Path != workspace {
		return false
	}
	return search == "" || strings.Contains(strings.ToLower(session.Title), search) ||
		strings.Contains(strings.ToLower(session.Workspace.Path), search) ||
		strings.Contains(strings.ToLower(session.Workspace.ProjectRoot), search)
}

func projectSessionPage(page *protocol.Page[protocol.Session], cursor string, limit int) (agent.SessionPage, error) {
	if page == nil {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned a nil page")
	}
	if len(page.Data) > limit {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned %d rows for limit %d", len(page.Data), limit)
	}
	if page.NextCursor != "" && page.NextCursor == cursor {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned its request cursor as the continuation")
	}
	result := agent.SessionPage{Items: make([]agent.Session, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		projected, err := projectSession(value)
		if err != nil {
			return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid session: %v", err)
		}
		result.Items = append(result.Items, projected)
	}
	if err := result.Validate(); err != nil {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid projection: %v", err)
	}
	return result, nil
}

func projectSession(value protocol.Session) (agent.Session, error) {
	projectedWorkspace, err := projectWorkspace(value.Workspace)
	if err != nil {
		return agent.Session{}, fmt.Errorf("runtime session %q: %w", value.ID, err)
	}
	status := agent.SessionStatus(value.Status)
	projected := agent.Session{
		ID: value.ID, Title: value.Title, Status: status, Model: value.Model,
		Workspace: projectedWorkspace, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
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
	return projectSessionResult("create session", "", created, err)
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
	return projectSessionResult("update session", input.SessionID, updated, err)
}

func (r *Runtime) ForkSession(ctx context.Context, input agent.ForkSession) (agent.Session, error) {
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	forked, err := r.sessionCatalog.ForkSession(ctx, protocol.ForkSessionRequest{
		SessionID: input.SessionID, FromRunID: input.FromRunID, Title: input.Title,
	}, options)
	projected, err := projectSessionResult("fork session", "", forked, err)
	if err != nil {
		return agent.Session{}, err
	}
	if projected.ID == input.SessionID {
		return agent.Session{}, runtimeContractViolation("fork session returned its source id %q", input.SessionID)
	}
	return projected, nil
}

func projectSessionResult(operation, expectedID string, result *protocol.Session, err error) (agent.Session, error) {
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if result == nil {
		return agent.Session{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected, err := projectSession(*result)
	if err != nil {
		return agent.Session{}, runtimeContractViolation("%s returned an invalid session: %v", operation, err)
	}
	if expectedID != "" && projected.ID != expectedID {
		return agent.Session{}, runtimeContractViolation("%s returned id %q for %q", operation, projected.ID, expectedID)
	}
	return projected, nil
}

func (r *Runtime) DeleteSession(ctx context.Context, input agent.DeleteSession) error {
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.sessionCatalog.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: input.SessionID}, options))
}
