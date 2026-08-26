package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	KnowledgeList   Name = "knowledge.list"
	KnowledgeGet    Name = "knowledge.get"
	KnowledgeUpdate Name = "knowledge.update"
)

func registerKnowledge(registry *Registry) {
	registry.Query(MethodMeta{
		Name: KnowledgeList, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, func(service interface {
		ListKnowledge(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
		return service.ListKnowledge(ctx, request)
	})

	registry.Query(MethodMeta{
		Name: KnowledgeGet, Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureKnowledge),
	}, func(service interface {
		GetKnowledge(context.Context, protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error)
	}, ctx context.Context, request protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
		return service.GetKnowledge(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: KnowledgeUpdate, Errors: []string{
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
