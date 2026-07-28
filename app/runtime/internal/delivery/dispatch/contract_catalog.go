package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerCatalog(r *Registry) {
	Unary(r, MethodMeta{Name: "providers.list", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Provider], error) {
			return d.api.ListProviders(ctx, in)
		})

	Unary(r, MethodMeta{
		Name:        "providers.configure",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
		return d.api.ConfigureProvider(ctx, in)
	})

	// The probe's verdict rides its own result (ProviderTestResult.error), so the
	// call succeeds even when the provider does not — nothing to replay-protect.
	Unary(r, MethodMeta{Name: "providers.test", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.TestProviderRequest) (*protocol.ProviderTestResult, error) {
			return d.api.TestProvider(ctx, in.Provider)
		})

	Unary(r, MethodMeta{Name: "models.list", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return d.api.ListModels(ctx, in)
		})

	Unary(r, MethodMeta{Name: "models.getUtilityRole", Stability: stable},
		func(d *Dispatcher, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return d.api.GetUtilityRole(ctx)
		})

	Unary(r, MethodMeta{
		Name:        "models.setUtilityRole",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
		return d.api.SetUtilityRole(ctx, in)
	})

	Unary(r, MethodMeta{Name: "models.getEmbeddingRole", Stability: stable},
		func(d *Dispatcher, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return d.api.GetEmbeddingRole(ctx)
		})

	Unary(r, MethodMeta{
		Name:        "models.setEmbeddingRole",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
		return d.api.SetEmbeddingRole(ctx, in)
	})

	Unary(r, MethodMeta{Name: "tools.list", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.ToolSpec], error) {
			return d.api.ListTools(ctx, in)
		})

	Unary(r, MethodMeta{
		Name:        "tools.invoke",
		Idempotency: IdempotencyReplayResponse,
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
		return d.api.InvokeTool(ctx, in)
	})
}

func registerUsage(r *Registry) {
	Unary(r, MethodMeta{
		Name:      "usage.session",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return d.api.SessionUsage(ctx, in.SessionID)
	})

	Unary(r, MethodMeta{Name: "usage.summary", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return d.api.UsageSummary(ctx, in)
		})
}

func registerMemory(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "memory.list",
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(featureMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
		return d.api.ListMemory(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "memory.get",
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(featureMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetMemoryRequest) (*protocol.MemoryEntry, error) {
		return d.api.GetMemory(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "memory.update",
		Idempotency:     IdempotencyReplayResponse,
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(featureMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UpdateMemoryRequest) error {
		return d.api.UpdateMemory(ctx, in)
	})
}

func registerAgentMemory(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "agentMemory.list",
		CapabilityRules: requires(featureAgentMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.AgentMemoryListRequest) (*protocol.AgentMemoryList, error) {
		return d.api.ListAgentMemory(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "agentMemory.review",
		CapabilityRules: requires(featureAgentMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.AgentMemoryReviewRequest) error {
		return d.api.ReviewAgentMemory(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "agentMemory.update",
		CapabilityRules: requires(featureAgentMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.AgentMemoryUpdateRequest) (*protocol.AgentMemoryItem, error) {
		return d.api.UpdateAgentMemory(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "agentMemory.delete",
		CapabilityRules: requires(featureAgentMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.AgentMemoryItemRequest) error {
		return d.api.DeleteAgentMemory(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "agentMemory.add",
		CapabilityRules: requires(featureAgentMemory),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
		return d.api.AddAgentMemory(ctx, in)
	})
}

func registerFeedback(r *Registry) {
	UnaryAck(r, MethodMeta{
		Name:        "feedback.create",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.FeedbackRequest) error {
		return d.api.CreateFeedback(ctx, in)
	})
}
