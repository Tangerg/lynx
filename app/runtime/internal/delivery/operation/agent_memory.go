package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerAgentMemory(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "agentMemory.list", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
		return service.ListAgentMemory(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "agentMemory.review", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.AgentMemoryReviewRequest) error {
		return service.ReviewAgentMemory(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "agentMemory.update", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return service.UpdateAgentMemory(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "agentMemory.delete", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.AgentMemoryItemRequest) error {
		return service.DeleteAgentMemory(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "agentMemory.add", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return service.AddAgentMemory(ctx, request)
	})
}
