package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerCodebase(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "codebase.search", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
		return service.CodebaseSearch(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "codebase.status", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
		return service.CodebaseStatus(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "codebase.reindex", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
		return service.CodebaseReindex(ctx, request)
	})
}
