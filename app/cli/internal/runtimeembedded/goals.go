package runtimeembedded

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/goal"
)

type goalBinding interface {
	GetGoal(context.Context, protocol.GoalRequest, embedded.CallOptions) (*protocol.Goal, error)
	StartGoal(context.Context, protocol.StartGoalRequest, embedded.CommandOptions) (*protocol.Goal, error)
	UpdateGoal(context.Context, protocol.UpdateGoalRequest, embedded.CommandOptions) (*protocol.Goal, error)
	ClearGoal(context.Context, protocol.GoalRequest, embedded.CommandOptions) error
	StopGoal(context.Context, protocol.GoalRequest, embedded.CommandOptions) (*protocol.Goal, error)
	ResumeGoal(context.Context, protocol.GoalRequest, embedded.CommandOptions) (*protocol.Goal, error)
}

func (r *Runtime) UpdateGoal(ctx context.Context, update goal.Update) (goal.Goal, error) {
	if err := update.Validate(); err != nil {
		return goal.Goal{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return goal.Goal{}, err
	}
	result, err := r.goals.UpdateGoal(ctx, protocol.UpdateGoalRequest{
		SessionID: update.SessionID,
		Objective: update.Objective,
	}, options)
	projected, err := projectGoalResult("update goal", update.SessionID, result, err)
	if err != nil {
		return goal.Goal{}, err
	}
	if err := update.ValidateResult(projected); err != nil {
		return goal.Goal{}, runtimeContractViolation("update goal returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Runtime) ClearGoal(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("clear goal: session id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	if err := r.goals.ClearGoal(ctx, protocol.GoalRequest{SessionID: sessionID}, options); err != nil {
		return classifyError(err)
	}
	return nil
}

var _ goal.Service = (*Runtime)(nil)

func (r *Runtime) GetGoal(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	if sessionID == "" {
		return goal.Goal{}, false, errors.New("get goal: session id is empty")
	}
	result, err := r.goals.GetGoal(ctx, protocol.GoalRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return goal.Goal{}, false, classifyError(err)
	}
	if result == nil {
		return goal.Goal{}, false, nil
	}
	projected, err := projectGoalResult("get goal", sessionID, result, nil)
	return projected, err == nil, err
}

func (r *Runtime) StartGoal(ctx context.Context, start goal.Start) (goal.Goal, error) {
	if err := start.Validate(); err != nil {
		return goal.Goal{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return goal.Goal{}, err
	}
	result, err := r.goals.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: start.SessionID, Objective: start.Objective,
		Provider: start.Provider, Model: start.Model,
		Budget: protocol.GoalBudget{MaxRuns: start.Budget.MaxRuns, MaxCostUSD: start.Budget.MaxCostUSD, MaxSteps: start.Budget.MaxSteps},
	}, options)
	projected, err := projectGoalResult("start goal", start.SessionID, result, err)
	if err != nil {
		return goal.Goal{}, err
	}
	if err := start.ValidateResult(projected); err != nil {
		return goal.Goal{}, runtimeContractViolation("start goal returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Runtime) StopGoal(ctx context.Context, sessionID string) (goal.Goal, error) {
	projected, err := r.changeGoal(ctx, "stop goal", sessionID, r.goals.StopGoal)
	if err != nil {
		return goal.Goal{}, err
	}
	if projected.Status == goal.Active {
		return goal.Goal{}, runtimeContractViolation("stop goal returned an active acknowledgement")
	}
	return projected, nil
}

func (r *Runtime) ResumeGoal(ctx context.Context, sessionID string) (goal.Goal, error) {
	projected, err := r.changeGoal(ctx, "resume goal", sessionID, r.goals.ResumeGoal)
	if err != nil {
		return goal.Goal{}, err
	}
	if projected.Status != goal.Active {
		return goal.Goal{}, runtimeContractViolation(
			"resume goal returned status %q, want %q",
			projected.Status,
			goal.Active,
		)
	}
	return projected, nil
}

func (r *Runtime) changeGoal(
	ctx context.Context,
	operation, sessionID string,
	change func(context.Context, protocol.GoalRequest, embedded.CommandOptions) (*protocol.Goal, error),
) (goal.Goal, error) {
	if sessionID == "" {
		return goal.Goal{}, fmt.Errorf("%s: session id is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return goal.Goal{}, err
	}
	result, err := change(ctx, protocol.GoalRequest{SessionID: sessionID}, options)
	return projectGoalResult(operation, sessionID, result, err)
}

func projectGoalResult(operation, expectedSessionID string, result *protocol.Goal, err error) (goal.Goal, error) {
	if err != nil {
		return goal.Goal{}, classifyError(err)
	}
	if result == nil {
		return goal.Goal{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected, err := projectGoal(*result)
	if err != nil {
		return goal.Goal{}, runtimeContractViolation("%s returned an invalid goal: %v", operation, err)
	}
	if projected.SessionID != expectedSessionID {
		return goal.Goal{}, runtimeContractViolation(
			"%s returned session %q for %q",
			operation,
			projected.SessionID,
			expectedSessionID,
		)
	}
	return projected, nil
}

func projectGoal(value protocol.Goal) (goal.Goal, error) {
	projected := goal.Goal{
		SessionID: value.SessionID, Objective: value.Objective, Status: goal.Status(value.Status),
		Provider: value.Provider, Model: value.Model,
		Budget:    goal.Budget{MaxRuns: value.Budget.MaxRuns, MaxCostUSD: value.Budget.MaxCostUSD, MaxSteps: value.Budget.MaxSteps},
		Used:      goal.Usage{Runs: value.Used.Runs, CostUSD: value.Used.CostUSD, Steps: value.Used.Steps},
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Reason != nil {
		projected.Reason = &goal.Reason{Code: goal.ReasonCode(value.Reason.Code), Detail: value.Reason.Detail}
	}
	if err := projected.Validate(); err != nil {
		return goal.Goal{}, fmt.Errorf("project goal: %w", err)
	}
	return projected, nil
}
