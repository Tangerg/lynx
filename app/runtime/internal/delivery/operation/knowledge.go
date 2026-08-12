package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerKnowledge(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "knowledge.list", Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return service.ListKnowledge(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "knowledge.get", Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.GetKnowledge(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "knowledge.update", Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrRevisionConflict.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.UpdateKnowledge(ctx, request)
	})
}
