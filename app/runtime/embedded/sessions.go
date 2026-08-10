package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListSessions returns one cursor page of Sessions.
func (r *Runtime) ListSessions(ctx context.Context, request protocol.PageQuery, options CallOptions) (*protocol.Page[protocol.Session], error) {
	return invoke[protocol.PageQuery, *protocol.Page[protocol.Session]](ctx, r, "sessions.list", request, callOptions(options))
}

// GetSession returns one Session by identity.
func (r *Runtime) GetSession(ctx context.Context, request protocol.GetSessionRequest, options CallOptions) (*protocol.Session, error) {
	return invoke[protocol.GetSessionRequest, *protocol.Session](ctx, r, "sessions.get", request, callOptions(options))
}

// CreateSession creates a Session.
func (r *Runtime) CreateSession(ctx context.Context, request protocol.CreateSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return invoke[protocol.CreateSessionRequest, *protocol.Session](ctx, r, "sessions.create", request, commandOptions(options))
}

// UpdateSession applies a revision-checked Session edit.
func (r *Runtime) UpdateSession(ctx context.Context, request protocol.UpdateSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return invoke[protocol.UpdateSessionRequest, *protocol.Session](ctx, r, "sessions.update", request, commandOptions(options))
}

// DeleteSession deletes a Session.
func (r *Runtime) DeleteSession(ctx context.Context, request protocol.DeleteSessionRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "sessions.delete", request, commandOptions(options))
}

// ForkSession creates a Session from an existing history boundary.
func (r *Runtime) ForkSession(ctx context.Context, request protocol.ForkSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return invoke[protocol.ForkSessionRequest, *protocol.Session](ctx, r, "sessions.fork", request, commandOptions(options))
}

// RollbackSession rewinds Session history and, when requested, its workspace.
func (r *Runtime) RollbackSession(ctx context.Context, request protocol.RollbackSessionRequest, options CommandOptions) (*protocol.RollbackSessionResponse, error) {
	return invoke[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, r, "sessions.rollback", request, commandOptions(options))
}

// ExportSession returns a portable Session artifact.
func (r *Runtime) ExportSession(ctx context.Context, request protocol.ExportSessionRequest, options CallOptions) (*protocol.ExportSessionResponse, error) {
	return invoke[protocol.ExportSessionRequest, *protocol.ExportSessionResponse](ctx, r, "sessions.export", request, callOptions(options))
}

// ImportSession restores a portable Session artifact.
func (r *Runtime) ImportSession(ctx context.Context, request protocol.ImportSessionRequest, options CommandOptions) (*protocol.ImportSessionResponse, error) {
	return invoke[protocol.ImportSessionRequest, *protocol.ImportSessionResponse](ctx, r, "sessions.import", request, commandOptions(options))
}
