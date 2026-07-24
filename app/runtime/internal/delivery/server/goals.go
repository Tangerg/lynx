package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// goals.* (API.md §7.14) — Goal mode: an autonomous loop that drives runs toward
// an objective until the model signals complete/blocked, a budget is spent, or
// the user stops it.

// goalRunner is the server's narrow view of the goal driver.
type goalRunner interface {
	Start(ctx context.Context, sessionID, objective, provider, model string, budget goal.Budget) (goal.Goal, error)
	Resume(ctx context.Context, sessionID string) (goal.Goal, error)
	Stop(ctx context.Context, sessionID string) (goal.Goal, error)
	Get(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

// StartGoal opens and begins driving a goal for the session (goals.start).
func (s *Server) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	if !s.features.goals {
		return nil, capabilityNotNegotiated("goals.start")
	}
	g, err := s.goals.Start(ctx, in.SessionID, in.Objective, in.Provider, in.Model, budgetFromWire(in.Budget))
	if err != nil {
		return nil, mapGoalErr(err, "goals.start")
	}
	return goalPtr(g)
}

// GetGoal returns the session's goal, or a nil result when it has none (goals.get).
func (s *Server) GetGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	if !s.features.goals {
		return nil, capabilityNotNegotiated("goals.get")
	}
	g, ok, err := s.goals.Get(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.get")
	}
	if !ok {
		return nil, nil
	}
	return goalPtr(g)
}

// StopGoal pauses the session's goal and stops the loop (goals.stop).
func (s *Server) StopGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	if !s.features.goals {
		return nil, capabilityNotNegotiated("goals.stop")
	}
	g, err := s.goals.Stop(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.stop")
	}
	return goalPtr(g)
}

// ResumeGoal re-activates a paused or blocked goal (goals.resume).
func (s *Server) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	if !s.features.goals {
		return nil, capabilityNotNegotiated("goals.resume")
	}
	g, err := s.goals.Resume(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.resume")
	}
	return goalPtr(g)
}

func mapGoalErr(err error, method string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, goals.ErrGoalActive):
		return fmt.Errorf("%w: a goal is already active for this session — stop it first", protocol.ErrSessionBusy)
	case errors.Is(err, goals.ErrNoGoal):
		return fmt.Errorf("%w: no goal for this session", protocol.ErrInvalidParams)
	default:
		return err
	}
}

func budgetFromWire(b protocol.GoalBudget) goal.Budget {
	return goal.Budget{MaxTurns: b.MaxTurns, MaxCostUSD: b.MaxCostUsd, MaxSteps: b.MaxSteps}
}

func goalPtr(g goal.Goal) (*protocol.Goal, error) {
	status, ok := goalStatusWire(g.Status)
	if !ok {
		return nil, fmt.Errorf("goals: unsupported status %q", g.Status)
	}
	w := protocol.Goal{
		SessionID: g.SessionID,
		Objective: g.Objective,
		Status:    status,
		Reason:    goalReason(g.Reason),
		Provider:  g.Provider,
		Model:     g.Model,
		Budget:    protocol.GoalBudget{MaxTurns: g.Budget.MaxTurns, MaxCostUsd: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps},
		Used:      protocol.GoalUsage{Turns: g.Used.Turns, CostUsd: g.Used.CostUSD, Steps: g.Used.Steps},
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
	return &w, nil
}

func goalStatusWire(status goal.Status) (protocol.GoalStatus, bool) {
	switch status {
	case goal.StatusActive:
		return protocol.GoalActive, true
	case goal.StatusPaused:
		return protocol.GoalPaused, true
	case goal.StatusBlocked:
		return protocol.GoalBlocked, true
	default:
		return "", false
	}
}

// goalReason owns the current wire's human-readable reason. The goal entity
// persists a typed cause plus an optional safe model/domain detail; infrastructure
// diagnostics never enter durable state or protocol output.
func goalReason(reason goal.Reason) string {
	switch reason.Cause {
	case goal.ReasonNone:
		return ""
	case goal.ReasonStoppedByUser:
		return "stopped by the user"
	case goal.ReasonRuntimeRestarted:
		return "the runtime restarted — resume to continue"
	case goal.ReasonRunStartFailed:
		return "could not start the next run"
	case goal.ReasonAwaitingInput:
		return "the run is waiting for your input"
	case goal.ReasonTerminalOutcomeMissing:
		return "the run ended without a terminal outcome"
	case goal.ReasonRunNotCompleted:
		if reason.Detail == "" {
			return "the run ended before completing the goal"
		}
		return "the run ended (" + reason.Detail + ")"
	case goal.ReasonTurnBudgetReached:
		return "reached the turn budget"
	case goal.ReasonCostBudgetReached:
		return "reached the cost budget"
	case goal.ReasonStepBudgetReached:
		return "reached the step budget"
	case goal.ReasonBlockedByModel:
		if reason.Detail != "" {
			return reason.Detail
		}
		return "the model reported that it is blocked"
	default:
		return "the goal stopped"
	}
}
