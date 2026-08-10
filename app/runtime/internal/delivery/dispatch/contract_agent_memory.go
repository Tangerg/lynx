package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerAgentMemory(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "agentMemory.list", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
		return router.api.ListAgentMemory(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "agentMemory.review", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.AgentMemoryReviewRequest) error {
		return router.api.ReviewAgentMemory(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "agentMemory.update", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return router.api.UpdateAgentMemory(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "agentMemory.delete", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.AgentMemoryItemRequest) error {
		return router.api.DeleteAgentMemory(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "agentMemory.add", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return router.api.AddAgentMemory(ctx, request)
	})
}
