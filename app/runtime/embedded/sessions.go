package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListSessions returns one cursor page of Sessions.
func (r *Runtime) ListSessions(ctx context.Context, request protocol.PageQuery, options CallOptions) (*protocol.Page[protocol.Session], error) {
	return r.invoke[protocol.PageQuery, *protocol.Page[protocol.Session]](ctx, operation.SessionsList, request, callOptions(options))
}

// GetSession returns one Session by identity.
func (r *Runtime) GetSession(ctx context.Context, request protocol.GetSessionRequest, options CallOptions) (*protocol.Session, error) {
	return r.invoke[protocol.GetSessionRequest, *protocol.Session](ctx, operation.SessionsGet, request, callOptions(options))
}

// GetSessionSnapshot returns one transactionally coherent mounted-session read.
func (r *Runtime) GetSessionSnapshot(ctx context.Context, request protocol.GetSessionSnapshotRequest, options CallOptions) (*protocol.SessionSnapshot, error) {
	return r.invoke[protocol.GetSessionSnapshotRequest, *protocol.SessionSnapshot](ctx, operation.SessionsSnapshot, request, callOptions(options))
}

// CreateSession creates a Session.
func (r *Runtime) CreateSession(ctx context.Context, request protocol.CreateSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return r.invoke[protocol.CreateSessionRequest, *protocol.Session](ctx, operation.SessionsCreate, request, commandOptions(options))
}

// UpdateSession applies a revision-checked Session edit.
func (r *Runtime) UpdateSession(ctx context.Context, request protocol.UpdateSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return r.invoke[protocol.UpdateSessionRequest, *protocol.Session](ctx, operation.SessionsUpdate, request, commandOptions(options))
}

// DeleteSession deletes a Session.
func (r *Runtime) DeleteSession(ctx context.Context, request protocol.DeleteSessionRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SessionsDelete, request, commandOptions(options))
}

// ForkSession creates a Session from an existing history boundary.
func (r *Runtime) ForkSession(ctx context.Context, request protocol.ForkSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return r.invoke[protocol.ForkSessionRequest, *protocol.Session](ctx, operation.SessionsFork, request, commandOptions(options))
}

// RollbackSession rewinds Session history and, when requested, its workspace.
func (r *Runtime) RollbackSession(ctx context.Context, request protocol.RollbackSessionRequest, options CommandOptions) (*protocol.RollbackSessionResponse, error) {
	return r.invoke[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, operation.SessionsRollback, request, commandOptions(options))
}

// ExportSession returns a portable Session artifact.
func (r *Runtime) ExportSession(ctx context.Context, request protocol.ExportSessionRequest, options CallOptions) (*protocol.ExportSessionResponse, error) {
	return r.invoke[protocol.ExportSessionRequest, *protocol.ExportSessionResponse](ctx, operation.SessionsExport, request, callOptions(options))
}

// ImportSession restores a portable Session artifact.
func (r *Runtime) ImportSession(ctx context.Context, request protocol.ImportSessionRequest, options CommandOptions) (*protocol.ImportSessionResponse, error) {
	return r.invoke[protocol.ImportSessionRequest, *protocol.ImportSessionResponse](ctx, operation.SessionsImport, request, commandOptions(options))
}
