package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerMCP(r *Registry) {
	Query(r, MethodMeta{
		Name:            "mcp.servers.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
		return d.api.ListMCPServers(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "mcp.servers.create",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CreateMCPServerRequest) (*protocol.McpServer, error) {
		return d.api.CreateMCPServer(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "mcp.servers.update",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UpdateMCPServerRequest) (*protocol.McpServer, error) {
		return d.api.UpdateMCPServer(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "mcp.servers.delete",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.DeleteMCPServer(ctx, in.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	Query(r, MethodMeta{
		Name:            "mcp.servers.test",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerCandidate) (*protocol.McpTestResult, error) {
		return d.api.TestMCPServer(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "mcp.tools.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
		return d.api.ListMCPTools(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "mcp.servers.reconnect",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.ReconnectMCPServer(ctx, in.Server)
	})

	CommandAck(r, MethodMeta{
		Name:            "mcp.servers.authorize",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.AuthorizeMCPServer(ctx, in.Server)
	})

}

func registerApproval(r *Registry) {
	Query(r, MethodMeta{Name: "approval.getMode", Stability: stable},
		func(d *Dispatcher, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return d.api.GetApprovalMode(ctx)
		})

	Command(r, MethodMeta{
		Name:      "approval.setMode",
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
		return d.api.SetApprovalMode(ctx, in)
	})

	Query(r, MethodMeta{Name: "approval.listRules", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return d.api.ListApprovalRules(ctx, in)
		})

	CommandAck(r, MethodMeta{
		Name:      "approval.forgetRule",
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ForgetApprovalRuleRequest) error {
		return d.api.ForgetApprovalRule(ctx, in)
	})
}

func registerSchedules(r *Registry) {
	Query(r, MethodMeta{
		Name:            "schedules.list",
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return d.api.ListSchedules(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "schedules.create",
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.CreateSchedule(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "schedules.update",
		Errors:          []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.UpdateSchedule(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "schedules.delete",
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.DeleteScheduleRequest) error {
		return d.api.DeleteSchedule(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "schedules.runNow",
		CapabilityRules: requires(protocol.FeatureSchedules),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return d.api.RunScheduleNow(ctx, in)
	})
}

func registerGoals(r *Registry) {
	Command(r, MethodMeta{
		Name:            "goals.start",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
		return d.api.StartGoal(ctx, in)
	})

	Query(r, MethodMeta{
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

	Command(r, MethodMeta{
		Name:            "goals.stop",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.StopGoal(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "goals.resume",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.ResumeGoal(ctx, in)
	})
}

func registerCodebase(r *Registry) {
	Query(r, MethodMeta{
		Name:            "codebase.search",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
		return d.api.CodebaseSearch(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "codebase.status",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
		return d.api.CodebaseStatus(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "codebase.reindex",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureCodebase),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
		return d.api.CodebaseReindex(ctx, in)
	})
}
