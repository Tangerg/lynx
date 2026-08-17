package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerCodebase(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "codebase.search", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service interface {
		CodebaseSearch(context.Context, protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error)
	}, ctx context.Context, request protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
		return service.CodebaseSearch(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "codebase.status", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service interface {
		CodebaseStatus(context.Context, protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error)
	}, ctx context.Context, request protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
		return service.CodebaseStatus(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "codebase.reindex", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase), Stability: stable,
	}, func(service interface {
		CodebaseReindex(context.Context, protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error)
	}, ctx context.Context, request protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
		return service.CodebaseReindex(ctx, request)
	})
}
