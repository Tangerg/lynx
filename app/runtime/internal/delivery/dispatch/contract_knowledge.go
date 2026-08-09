package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerKnowledge(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "knowledge.list", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return router.api.ListKnowledge(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "knowledge.get", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return router.api.GetKnowledge(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "knowledge.update", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.UpdateKnowledgeRequest) error {
		return router.api.UpdateKnowledge(ctx, request)
	})
}
