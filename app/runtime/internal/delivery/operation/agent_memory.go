package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	AgentMemoryList   Name = "agentMemory.list"
	AgentMemoryReview Name = "agentMemory.review"
	AgentMemoryUpdate Name = "agentMemory.update"
	AgentMemoryDelete Name = "agentMemory.delete"
	AgentMemoryAdd    Name = "agentMemory.add"
)

func registerAgentMemory(registry *Registry) {
	registry.Query(MethodMeta{
		Name: AgentMemoryList, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		ListAgentMemory(context.Context, protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error)
	}, ctx context.Context, request protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
		return service.ListAgentMemory(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name: AgentMemoryReview, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest) error
	}, ctx context.Context, request protocol.AgentMemoryReviewRequest) error {
		return service.ReviewAgentMemory(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: AgentMemoryUpdate, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error)
	}, ctx context.Context, request protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return service.UpdateAgentMemory(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name: AgentMemoryDelete, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		DeleteAgentMemory(context.Context, protocol.AgentMemoryItemRequest) error
	}, ctx context.Context, request protocol.AgentMemoryItemRequest) error {
		return service.DeleteAgentMemory(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: AgentMemoryAdd, CapabilityRules: requires(protocol.FeatureAgentMemory),
	}, func(service interface {
		AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error)
	}, ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return service.AddAgentMemory(ctx, request)
	})
}
