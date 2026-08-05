// Package executionctx carries application-owned execution facts through Go
// context. Runtime seeds immutable host scope and ephemeral Run metadata at the
// root execution boundary; child Agents inherit them naturally because context
// propagation is part of execution, while Agent blackboards remain exclusively
// planner/action working state.
package executionctx

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type scopeKey struct{}

// WithScope returns a context carrying scope. The value is immutable and safe
// to share across the complete delegation tree.
func WithScope(ctx context.Context, scope execution.ExecutionScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, scopeKey{}, scope)
}

// Scope returns the application scope attached at the root Run boundary.
func Scope(ctx context.Context) (execution.ExecutionScope, bool) {
	if ctx == nil {
		return execution.ExecutionScope{}, false
	}
	scope, ok := ctx.Value(scopeKey{}).(execution.ExecutionScope)
	return scope, ok
}

// CWD returns the execution workspace, falling back when the Run is
// unattached. Every cwd-dependent adapter reads this single host seam.
func CWD(ctx context.Context, fallback string) string {
	if scope, ok := Scope(ctx); ok && scope.Cwd != "" {
		return scope.Cwd
	}
	return fallback
}

// WorkspaceCWD returns the persistent session workspace, falling back when the
// Run is unattached. Unlike [CWD], it never points at an isolated scratch copy.
func WorkspaceCWD(ctx context.Context, fallback string) string {
	if scope, ok := Scope(ctx); ok && scope.WorkspaceCwd != "" {
		return scope.WorkspaceCwd
	}
	return fallback
}

// Isolated reports whether the running Run is in an isolated session
// so shell execution applies the host's OS jail.
func Isolated(ctx context.Context) bool {
	scope, ok := Scope(ctx)
	return ok && scope.Isolated
}

// GoalLeaseID reports the goal incarnation this run was launched under
// and whether it was set.
func GoalLeaseID(ctx context.Context) (string, bool) {
	if scope, ok := Scope(ctx); ok && scope.GoalLeaseID != "" {
		return scope.GoalLeaseID, true
	}
	return "", false
}

// SessionID returns the owning product session, or empty for an unattached
// smoke Run.
func SessionID(ctx context.Context) string {
	if scope, ok := Scope(ctx); ok {
		return scope.SessionID
	}
	return ""
}
