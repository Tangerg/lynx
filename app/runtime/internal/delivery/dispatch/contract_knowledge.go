package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerKnowledge(r *Registry) {
	Query(r, MethodMeta{
		Name: "knowledge.list", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return d.api.ListKnowledge(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "knowledge.get", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return d.api.GetKnowledge(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name: "knowledge.update", Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureKnowledge), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.UpdateKnowledgeRequest) error {
		return d.api.UpdateKnowledge(ctx, in)
	})
}
