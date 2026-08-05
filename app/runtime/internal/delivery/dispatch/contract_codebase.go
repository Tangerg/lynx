package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerCodebase(r *Registry) {
	Query(r, MethodMeta{
		Name: "codebase.search", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
		return d.api.CodebaseSearch(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "codebase.status", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
		return d.api.CodebaseStatus(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "codebase.reindex", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
		return d.api.CodebaseReindex(ctx, in)
	})
}
