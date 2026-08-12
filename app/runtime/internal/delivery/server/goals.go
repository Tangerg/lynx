package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// goals.* (API.md §7.14) — Goal mode: an autonomous loop that drives runs toward
// an objective until the model signals complete/blocked, a budget is spent, or
// the user stops it.

type goalUseCases interface {
	Start(ctx context.Context, sessionID, objective string, selection modelref.Selection, budget goal.Budget, capabilities run.Capabilities) (goal.Goal, error)
	Resume(ctx context.Context, sessionID string, caller run.Capabilities) (goal.Goal, error)
	Stop(ctx context.Context, sessionID string) (goal.Goal, error)
	Current(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// StartGoal opens and begins driving a goal for the session (goals.start).
func (s *Server) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	selection, err := modelref.New(in.Provider, in.Model)
	if err != nil {
		return nil, mapGoalErr(err, "goals.start")
	}
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Start(ctx, in.SessionID, in.Objective, selection, budgetFromWire(in.Budget), capabilities)
	if err != nil {
		return nil, mapGoalErr(err, "goals.start")
	}
	return presentGoal(g)
}

// GetGoal returns the session's goal, or a nil result when it has none (goals.get).
func (s *Server) GetGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, ok, err := s.goals.Current(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.get")
	}
	if !ok {
		return nil, nil
	}
	return presentGoal(g)
}

// StopGoal pauses the session's goal and stops the loop (goals.stop).
func (s *Server) StopGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, err := s.goals.Stop(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.stop")
	}
	return presentGoal(g)
}

// ResumeGoal re-activates a paused or blocked goal (goals.resume).
func (s *Server) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.goals.Resume(ctx, in.SessionID, caller)
	if uncovered, ok := errors.AsType[*goals.InsufficientCapabilitiesError](err); ok {
		return nil, capabilityGap(uncovered.Missing)
	}
	if err != nil {
		return nil, mapGoalErr(err, "goals.resume")
	}
	return presentGoal(g)
}

func mapGoalErr(err error, method string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, goals.ErrUnavailable):
		return capabilityNotNegotiated(method)
	case errors.Is(err, goals.ErrGoalActive):
		return fmt.Errorf("%w: a goal is already active for this session — stop it first", protocol.ErrSessionBusy)
	case errors.Is(err, goals.ErrNoGoal):
		return fmt.Errorf("%w: no goal for this session", protocol.ErrInvalidParams)
	case errors.Is(err, goal.ErrBudgetExhausted):
		return fmt.Errorf("%w: goal budget is exhausted; start a new goal to change it", protocol.ErrInvalidParams)
	case errors.Is(err, goal.ErrNotResumable):
		return fmt.Errorf("%w: this goal is not resumable", protocol.ErrInvalidParams)
	case errors.Is(err, modelref.ErrIncomplete):
		return protocol.ErrInvalidParams
	default:
		return err
	}
}

func budgetFromWire(b protocol.GoalBudget) goal.Budget {
	return goal.Budget{MaxRuns: b.MaxRuns, MaxCostUSD: b.MaxCostUSD, MaxSteps: b.MaxSteps}
}

func presentGoal(g goal.Goal) (*protocol.Goal, error) {
	status, ok := presentGoalStatus(g.Status)
	if !ok {
		return nil, fmt.Errorf("goals: unsupported status %q", g.Status)
	}
	reason, err := presentGoalReason(g.Reason)
	if err != nil {
		return nil, err
	}
	w := protocol.Goal{
		SessionID: g.SessionID,
		Objective: g.Objective,
		Status:    status,
		Reason:    reason,
		Provider:  g.ModelSelection.Provider(),
		Model:     g.ModelSelection.Model(),
		Budget:    protocol.GoalBudget{MaxRuns: g.Budget.MaxRuns, MaxCostUSD: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps},
		Used:      protocol.GoalUsage{Runs: g.Used.Runs, CostUSD: g.Used.CostUSD, Steps: g.Used.Steps},
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
	return &w, nil
}

func presentGoalStatus(status goal.Status) (protocol.GoalStatus, bool) {
	switch status {
	case goal.StatusActive:
		return protocol.GoalActive, true
	case goal.StatusPaused:
		return protocol.GoalPaused, true
	case goal.StatusBlocked:
		return protocol.GoalBlocked, true
	case goal.StatusComplete:
		return protocol.GoalCompleting, true
	default:
		return "", false
	}
}

func presentGoalReason(reason goal.Reason) (*protocol.GoalReason, error) {
	var code protocol.GoalReasonCode
	switch reason.Code {
	case goal.ReasonNone:
		return nil, nil
	case goal.ReasonStoppedByUser:
		code = protocol.GoalReasonStoppedByUser
	case goal.ReasonRuntimeRestarted:
		code = protocol.GoalReasonRuntimeRestarted
	case goal.ReasonRunStartFailed:
		code = protocol.GoalReasonRunStartFailed
	case goal.ReasonAwaitingInput:
		code = protocol.GoalReasonAwaitingInput
	case goal.ReasonTerminalOutcomeMissing:
		code = protocol.GoalReasonTerminalOutcomeMissing
	case goal.ReasonRunNotCompleted:
		code = protocol.GoalReasonRunNotCompleted
	case goal.ReasonRunBudgetReached:
		code = protocol.GoalReasonRunBudgetReached
	case goal.ReasonCostBudgetReached:
		code = protocol.GoalReasonCostBudgetReached
	case goal.ReasonStepBudgetReached:
		code = protocol.GoalReasonStepBudgetReached
	case goal.ReasonBlockedByModel:
		code = protocol.GoalReasonBlockedByModel
	default:
		return nil, fmt.Errorf("goals: unsupported reason code %q", reason.Code)
	}
	return &protocol.GoalReason{Code: code, Detail: reason.Detail}, nil
}
