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

// goalRunner is the server's narrow view of the goal driver. A disabled build
// injects goalsUnavailable so goals.* report capability_not_negotiated.
type goalRunner interface {
	Start(ctx context.Context, sessionID, objective, provider, model string, budget goal.Budget) (goal.Goal, error)
	Resume(ctx context.Context, sessionID string) (goal.Goal, error)
	Stop(ctx context.Context, sessionID string) (goal.Goal, error)
	Get(ctx context.Context, sessionID string) (goal.Goal, bool, error)
}

var errGoalsDisabled = errors.New("goals: disabled")

type goalsUnavailable struct{}

func (goalsUnavailable) Start(context.Context, string, string, string, string, goal.Budget) (goal.Goal, error) {
	return goal.Goal{}, errGoalsDisabled
}
func (goalsUnavailable) Resume(context.Context, string) (goal.Goal, error) {
	return goal.Goal{}, errGoalsDisabled
}
func (goalsUnavailable) Stop(context.Context, string) (goal.Goal, error) {
	return goal.Goal{}, errGoalsDisabled
}
func (goalsUnavailable) Get(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, errGoalsDisabled
}

func goalRunnerOrDisabled(d goalRunner) goalRunner {
	if d == nil {
		return goalsUnavailable{}
	}
	return d
}

// StartGoal opens and begins driving a goal for the session (goals.start).
func (s *Server) StartGoal(ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
	g, err := s.goals.Start(ctx, in.SessionID, in.Objective, in.Provider, in.Model, budgetFromWire(in.Budget))
	if err != nil {
		return nil, mapGoalErr(err, "goals.start")
	}
	return goalPtr(g), nil
}

// GetGoal returns the session's goal, or a nil result when it has none (goals.get).
func (s *Server) GetGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, ok, err := s.goals.Get(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.get")
	}
	if !ok {
		return nil, nil
	}
	return goalPtr(g), nil
}

// StopGoal pauses the session's goal and stops the loop (goals.stop).
func (s *Server) StopGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, err := s.goals.Stop(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.stop")
	}
	return goalPtr(g), nil
}

// ResumeGoal re-activates a paused or blocked goal (goals.resume).
func (s *Server) ResumeGoal(ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
	g, err := s.goals.Resume(ctx, in.SessionID)
	if err != nil {
		return nil, mapGoalErr(err, "goals.resume")
	}
	return goalPtr(g), nil
}

func mapGoalErr(err error, method string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, errGoalsDisabled):
		return capabilityNotNegotiated(method)
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

func goalPtr(g goal.Goal) *protocol.Goal {
	w := protocol.Goal{
		SessionID: g.SessionID,
		Objective: g.Objective,
		Status:    string(g.Status),
		Reason:    goalReason(g.Reason),
		Provider:  g.Provider,
		Model:     g.Model,
		Budget:    protocol.GoalBudget{MaxTurns: g.Budget.MaxTurns, MaxCostUsd: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps},
		Used:      protocol.GoalUsage{Turns: g.Used.Turns, CostUsd: g.Used.CostUSD, Steps: g.Used.Steps},
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
	return &w
}

// goalReason owns the current wire's human-readable reason. The goal entity
// persists a typed cause plus raw detail, so model and infrastructure data do
// not become presentation text in the domain or application layers.
func goalReason(reason goal.Reason) string {
	switch reason.Cause {
	case goal.ReasonNone:
		return ""
	case goal.ReasonStoppedByUser:
		return "stopped by the user"
	case goal.ReasonRuntimeRestarted:
		return "the runtime restarted — resume to continue"
	case goal.ReasonRunStartFailed:
		if reason.Detail == "" {
			return "could not start the next run"
		}
		return "could not start the next run: " + reason.Detail
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
