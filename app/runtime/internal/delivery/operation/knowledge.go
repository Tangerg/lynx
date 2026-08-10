package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerKnowledge(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "knowledge.list", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return service.ListKnowledge(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "knowledge.get", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.GetKnowledge(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "knowledge.update", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.UpdateKnowledgeRequest) error {
		return service.UpdateKnowledge(ctx, request)
	})
}
