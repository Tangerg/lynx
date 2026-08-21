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
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, func(service interface {
		ListKnowledge(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return service.ListKnowledge(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "knowledge.get", Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, func(service interface {
		GetKnowledge(context.Context, protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error)
	}, ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.GetKnowledge(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "knowledge.update", Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrRevisionConflict.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, func(service interface {
		UpdateKnowledge(context.Context, protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error)
	}, ctx context.Context, request protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.UpdateKnowledge(ctx, request)
	})
}
