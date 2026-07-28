package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerMCP(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "mcp.servers.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
		return d.api.ListMCPServers(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "mcp.tools.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
		return d.api.ListMCPTools(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "mcp.servers.reconnect",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.ReconnectMCPServer(ctx, in.Server)
	})

	UnaryAck(r, MethodMeta{
		Name:            "mcp.servers.authorize",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.AuthorizeMCPServer(ctx, in.Server)
	})

	Unary(r, MethodMeta{
		Name:            "mcp.configs.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.McpServerConfig], error) {
		return d.api.ListMCPServerConfigs(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "mcp.configs.configure",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpServerConfig, error) {
		return d.api.ConfigureMCPServer(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "mcp.configs.remove",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.RemoveMCPServerRequest) error {
		return d.api.RemoveMCPServer(ctx, in.Name)
	})

	UnaryAck(r, MethodMeta{
		Name:            "mcp.configs.setEnabled",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SetMCPEnabledRequest) error {
		return d.api.SetMCPServerEnabled(ctx, in)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	Unary(r, MethodMeta{
		Name:            "mcp.configs.test",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpTestResult, error) {
		return d.api.TestMCPServer(ctx, in)
	})
}

func registerApproval(r *Registry) {
	Unary(r, MethodMeta{Name: "approval.getMode", Stability: stable},
		func(d *Dispatcher, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return d.api.GetApprovalMode(ctx)
		})

	Unary(r, MethodMeta{
		Name:        "approval.setMode",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
		return d.api.SetApprovalMode(ctx, in)
	})

	Unary(r, MethodMeta{Name: "approval.listRules", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return d.api.ListApprovalRules(ctx, in)
		})

	UnaryAck(r, MethodMeta{
		Name:        "approval.forgetRule",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ForgetApprovalRuleRequest) error {
		return d.api.ForgetApprovalRule(ctx, in)
	})
}

func registerSchedules(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "schedules.list",
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return d.api.ListSchedules(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "schedules.create",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.CreateSchedule(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "schedules.update",
		Idempotency:     IdempotencyReplayResponse,
		Errors:          []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.UpdateSchedule(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "schedules.delete",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.DeleteScheduleRequest) error {
		return d.api.DeleteSchedule(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "schedules.runNow",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return d.api.RunScheduleNow(ctx, in)
	})
}

func registerGoals(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "goals.start",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
		return d.api.StartGoal(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:   "goals.get",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error — API.md §7.14 answers null, so the
		// published result has to admit it.
		ResultNullable:  true,
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.GetGoal(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "goals.stop",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.StopGoal(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "goals.resume",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.ResumeGoal(ctx, in)
	})
}

func registerCodebase(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "codebase.search",
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
		return d.api.CodebaseSearch(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "codebase.status",
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
		return d.api.CodebaseStatus(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "codebase.reindex",
		Idempotency:     IdempotencyReplayResponse,
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
		return d.api.CodebaseReindex(ctx, in)
	})
}
