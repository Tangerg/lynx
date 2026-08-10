package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetApprovalMode returns the Runtime approval mode.
func (r *Runtime) GetApprovalMode(ctx context.Context, options CallOptions) (*protocol.ApprovalModeResult, error) {
	return invoke[struct{}, *protocol.ApprovalModeResult](ctx, r, "approval.getMode", struct{}{}, callOptions(options))
}

// SetApprovalMode changes the Runtime approval mode.
func (r *Runtime) SetApprovalMode(ctx context.Context, request protocol.SetApprovalModeRequest, options CommandOptions) (*protocol.ApprovalModeResult, error) {
	return invoke[protocol.SetApprovalModeRequest, *protocol.ApprovalModeResult](ctx, r, "approval.setMode", request, commandOptions(options))
}

// ListApprovalRules returns remembered approval rules.
func (r *Runtime) ListApprovalRules(ctx context.Context, request protocol.ListApprovalRulesRequest, options CallOptions) (*protocol.ListApprovalRulesResult, error) {
	return invoke[protocol.ListApprovalRulesRequest, *protocol.ListApprovalRulesResult](ctx, r, "approval.listRules", request, callOptions(options))
}

// ForgetApprovalRule removes one remembered approval rule.
func (r *Runtime) ForgetApprovalRule(ctx context.Context, request protocol.ForgetApprovalRuleRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "approval.forgetRule", request, commandOptions(options))
}

// ListSchedules returns one cursor page of schedules.
func (r *Runtime) ListSchedules(ctx context.Context, request protocol.PageQuery, options CallOptions) (*protocol.Page[protocol.Schedule], error) {
	return invoke[protocol.PageQuery, *protocol.Page[protocol.Schedule]](ctx, r, "schedules.list", request, callOptions(options))
}

// CreateSchedule creates a schedule.
func (r *Runtime) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return invoke[protocol.CreateScheduleRequest, *protocol.Schedule](ctx, r, "schedules.create", request, commandOptions(options))
}

// UpdateSchedule applies a revision-checked schedule edit.
func (r *Runtime) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return invoke[protocol.UpdateScheduleRequest, *protocol.Schedule](ctx, r, "schedules.update", request, commandOptions(options))
}

// DeleteSchedule deletes a schedule.
func (r *Runtime) DeleteSchedule(ctx context.Context, request protocol.DeleteScheduleRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "schedules.delete", request, commandOptions(options))
}

// RunScheduleNow fires a schedule without advancing its cron cursor.
func (r *Runtime) RunScheduleNow(ctx context.Context, request protocol.RunScheduleNowRequest, options CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	return invoke[protocol.RunScheduleNowRequest, *protocol.RunScheduleNowResponse](ctx, r, "schedules.runNow", request, commandOptions(options))
}

// StartGoal starts autonomous Goal pursuit for a Session.
func (r *Runtime) StartGoal(ctx context.Context, request protocol.StartGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.StartGoalRequest, *protocol.Goal](ctx, r, "goals.start", request, commandOptions(options))
}

// GetGoal returns the Session's current Goal, or nil when none exists.
func (r *Runtime) GetGoal(ctx context.Context, request protocol.GoalRequest, options CallOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.get", request, callOptions(options))
}

// StopGoal stops autonomous Goal pursuit.
func (r *Runtime) StopGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.stop", request, commandOptions(options))
}

// ResumeGoal resumes paused Goal pursuit.
func (r *Runtime) ResumeGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.resume", request, commandOptions(options))
}

// GetSessionUsage returns usage accumulated by one Session.
func (r *Runtime) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options CallOptions) (*protocol.Usage, error) {
	return invoke[protocol.SessionUsageRequest, *protocol.Usage](ctx, r, "usage.session", request, callOptions(options))
}

// GetUsageSummary returns aggregated Runtime usage.
func (r *Runtime) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options CallOptions) (*protocol.UsageSummary, error) {
	return invoke[protocol.UsageSummaryRequest, *protocol.UsageSummary](ctx, r, "usage.summary", request, callOptions(options))
}

// ListAgentMemory returns curated Agent memory and review candidates.
func (r *Runtime) ListAgentMemory(ctx context.Context, request protocol.AgentMemoryListRequest, options CallOptions) (*protocol.AgentMemoryList, error) {
	return invoke[protocol.AgentMemoryListRequest, *protocol.AgentMemoryList](ctx, r, "agentMemory.list", request, callOptions(options))
}

// ReviewAgentMemory accepts or rejects an Agent memory candidate.
func (r *Runtime) ReviewAgentMemory(ctx context.Context, request protocol.AgentMemoryReviewRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "agentMemory.review", request, commandOptions(options))
}

// UpdateAgentMemory updates one curated Agent memory item.
func (r *Runtime) UpdateAgentMemory(ctx context.Context, request protocol.AgentMemoryUpdateRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return invoke[protocol.AgentMemoryUpdateRequest, *protocol.AgentMemoryItem](ctx, r, "agentMemory.update", request, commandOptions(options))
}

// DeleteAgentMemory deletes one curated Agent memory item.
func (r *Runtime) DeleteAgentMemory(ctx context.Context, request protocol.AgentMemoryItemRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "agentMemory.delete", request, commandOptions(options))
}

// AddAgentMemory adds one curated Agent memory item.
func (r *Runtime) AddAgentMemory(ctx context.Context, request protocol.AgentMemoryAddRequest, options CommandOptions) (*protocol.AgentMemoryItem, error) {
	return invoke[protocol.AgentMemoryAddRequest, *protocol.AgentMemoryItem](ctx, r, "agentMemory.add", request, commandOptions(options))
}

// CreateFeedback records one quality signal.
func (r *Runtime) CreateFeedback(ctx context.Context, request protocol.FeedbackRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "feedback.create", request, commandOptions(options))
}
