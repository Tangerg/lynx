package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerAgentMemory(r *Registry) {
	Query(r, MethodMeta{
		Name: "agentMemory.list", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
		return d.api.ListAgentMemory(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name: "agentMemory.review", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.AgentMemoryReviewRequest) error {
		return d.api.ReviewAgentMemory(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "agentMemory.update", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return d.api.UpdateAgentMemory(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name: "agentMemory.delete", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.AgentMemoryItemRequest) error {
		return d.api.DeleteAgentMemory(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "agentMemory.add", CapabilityRules: requires(protocol.FeatureAgentMemory), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return d.api.AddAgentMemory(ctx, in)
	})
}
