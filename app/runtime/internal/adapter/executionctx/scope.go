// Package executionctx carries application-owned execution facts through Go
// context. Runtime seeds immutable host scope and ephemeral Run metadata at the
// root execution boundary; child Agents inherit them naturally because context
// propagation is part of execution, while Agent blackboards remain exclusively
// planner/action working state.
package executionctx

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
)

type scopeKey struct{}
type runCapabilitiesKey struct{}
type modelSelectionKey struct{}

// WithScope returns a context carrying scope. The value is immutable and safe
// to share across the complete delegation tree.
func WithScope(ctx context.Context, scope runs.ExecutionScope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// Scope returns the application scope attached at the root Run boundary.
func Scope(ctx context.Context) (runs.ExecutionScope, bool) {
	if ctx == nil {
		return runs.ExecutionScope{}, false
	}
	scope, ok := ctx.Value(scopeKey{}).(runs.ExecutionScope)
	return scope, ok
}

// WithRunCapabilities carries the frozen product Run contract through tool
// execution without placing Runtime policy inside Agent Framework state.
func WithRunCapabilities(ctx context.Context, capabilities run.Capabilities) context.Context {
	return context.WithValue(ctx, runCapabilitiesKey{}, capabilities.Clone())
}

// RunCapabilities returns an ownership-isolated frozen Run contract.
func RunCapabilities(ctx context.Context) (run.Capabilities, bool) {
	if ctx == nil {
		return run.Capabilities{}, false
	}
	capabilities, ok := ctx.Value(runCapabilitiesKey{}).(run.Capabilities)
	return capabilities.Clone(), ok
}

// WithModelSelection carries the root Run's exact immutable model choice
// through model-invoked product tools.
func WithModelSelection(ctx context.Context, selection modelref.Selection) context.Context {
	return context.WithValue(ctx, modelSelectionKey{}, selection)
}

// ModelSelection returns the exact root Run model choice.
func ModelSelection(ctx context.Context) (modelref.Selection, bool) {
	if ctx == nil {
		return modelref.Selection{}, false
	}
	selection, ok := ctx.Value(modelSelectionKey{}).(modelref.Selection)
	return selection, ok
}

// CWD returns the execution workspace, falling back when the Run is
// unattached. Every cwd-dependent adapter reads this single host seam.
func CWD(ctx context.Context, fallback string) string {
	if scope, ok := Scope(ctx); ok && scope.CWD != "" {
		return scope.CWD
	}
	return fallback
}

// WorkspaceCWD returns the persistent session workspace, falling back when the
// Run is unattached. Unlike [CWD], it never points at an isolated scratch copy.
func WorkspaceCWD(ctx context.Context, fallback string) string {
	if scope, ok := Scope(ctx); ok && scope.WorkspaceCWD != "" {
		return scope.WorkspaceCWD
	}
	return fallback
}

// Isolated reports whether the running Run is in an isolated session
// so shell execution applies the host's OS jail.
func Isolated(ctx context.Context) bool {
	scope, ok := Scope(ctx)
	return ok && scope.Isolated
}

// GoalIncarnationID reports the goal incarnation this run was launched under
// and whether it was set.
func GoalIncarnationID(ctx context.Context) (string, bool) {
	if scope, ok := Scope(ctx); ok && scope.GoalIncarnationID != "" {
		return scope.GoalIncarnationID, true
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
