// Package stateflow owns Session-scoped Plan, Goal and Interrupt use cases.
package stateflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/state"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	GetSession(context.Context, session.ID) (session.Session, error)
	GetPlan(context.Context, string) (protocol.Plan, error)
	PutPlan(context.Context, protocol.Plan, uint64) error
	GetGoal(context.Context, string) (protocol.Goal, uint64, error)
	PutGoal(context.Context, protocol.Goal, uint64, *uint64) error
	DeleteGoal(context.Context, string) error
	ListInterruptSets(context.Context, string, string) ([]protocol.PendingInterruptSet, error)
}

type Service struct {
	store Store
	now func() time.Time
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("stateflow: store is required")
	}
	return &Service{store: store, now: time.Now}, nil
}

func (service *Service) Plan(ctx context.Context, request protocol.GetPlanRequest) (*protocol.Plan, error) {
	value, err := service.store.GetPlan(ctx, request.SessionID)
	if errors.Is(err, state.ErrNotFound) {
		return nil, protocol.ErrSessionNotFound
	}
	return &value, err
}

func (service *Service) Interrupts(ctx context.Context, request protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
	if request.SessionID == "" && request.RootRunID == "" {
		return nil, fmt.Errorf("%w: sessionId or rootRunId is required", protocol.ErrInvalidParams)
	}
	values, err := service.store.ListInterruptSets(ctx, request.SessionID, request.RootRunID)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(values), nil
}

func (service *Service) StartGoal(ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
	if strings.TrimSpace(request.Objective) == "" || request.Budget.MaxRuns < 0 || request.Budget.MaxCostUSD < 0 || request.Budget.MaxSteps < 0 || (request.Provider == "") != (request.Model == "") {
		return nil, fmt.Errorf("%w: invalid goal", protocol.ErrInvalidParams)
	}
	if _, err := service.store.GetSession(ctx, session.ID(request.SessionID)); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, protocol.ErrSessionNotFound
		}
		return nil, err
	}
	if _, _, err := service.store.GetGoal(ctx, request.SessionID); err == nil {
		return nil, fmt.Errorf("%w: a goal already exists", protocol.ErrSessionBusy)
	} else if !errors.Is(err, state.ErrNotFound) {
		return nil, err
	}
	now := service.now().UTC()
	value := protocol.Goal{
		SessionID: request.SessionID, Objective: strings.TrimSpace(request.Objective), Status: protocol.GoalActive,
		Provider: request.Provider, Model: request.Model, Budget: request.Budget,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := service.store.PutGoal(ctx, value, 1, nil); err != nil {
		return nil, err
	}
	return &value, nil
}

func (service *Service) UpdateGoal(ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	if strings.TrimSpace(request.Objective) == "" {
		return nil, fmt.Errorf("%w: objective is required", protocol.ErrInvalidParams)
	}
	value, incarnation, err := service.store.GetGoal(ctx, request.SessionID)
	if err != nil {
		return nil, service.goalError(err)
	}
	if value.Status == protocol.GoalCompleting {
		return nil, fmt.Errorf("%w: completing goal cannot be edited", protocol.ErrInvalidParams)
	}
	value.Objective = strings.TrimSpace(request.Objective)
	value.UpdatedAt = service.now().UTC()
	next := incarnation + 1
	if err := service.store.PutGoal(ctx, value, next, &incarnation); err != nil {
		return nil, service.goalError(err)
	}
	return &value, nil
}

func (service *Service) GetGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, _, err := service.store.GetGoal(ctx, request.SessionID)
	if errors.Is(err, state.ErrNotFound) {
		return nil, nil
	}
	return &value, err
}

func (service *Service) StopGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, incarnation, err := service.store.GetGoal(ctx, request.SessionID)
	if err != nil {
		return nil, service.goalError(err)
	}
	if value.Status != protocol.GoalActive {
		return nil, fmt.Errorf("%w: goal is not active", protocol.ErrInvalidParams)
	}
	value.Status = protocol.GoalPaused
	value.Reason = &protocol.GoalReason{Code: protocol.GoalReasonStoppedByUser}
	value.UpdatedAt = service.now().UTC()
	if err := service.store.PutGoal(ctx, value, incarnation, &incarnation); err != nil {
		return nil, service.goalError(err)
	}
	return &value, nil
}

func (service *Service) ResumeGoal(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, incarnation, err := service.store.GetGoal(ctx, request.SessionID)
	if err != nil {
		return nil, service.goalError(err)
	}
	if value.Status != protocol.GoalPaused && value.Status != protocol.GoalBlocked {
		return nil, fmt.Errorf("%w: goal is not resumable", protocol.ErrInvalidParams)
	}
	if exhausted(value) {
		return nil, fmt.Errorf("%w: goal budget is exhausted", protocol.ErrInvalidParams)
	}
	value.Status = protocol.GoalActive
	value.Reason = nil
	value.UpdatedAt = service.now().UTC()
	if err := service.store.PutGoal(ctx, value, incarnation, &incarnation); err != nil {
		return nil, service.goalError(err)
	}
	return &value, nil
}

func (service *Service) ClearGoal(ctx context.Context, request protocol.GoalRequest) error {
	err := service.store.DeleteGoal(ctx, request.SessionID)
	return service.goalError(err)
}

func (service *Service) goalError(err error) error {
	if errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("%w: no goal for session", protocol.ErrInvalidParams)
	}
	if errors.Is(err, state.ErrConflict) {
		return protocol.ErrRevisionConflict
	}
	return err
}

func exhausted(value protocol.Goal) bool {
	return value.Budget.MaxRuns > 0 && value.Used.Runs >= value.Budget.MaxRuns ||
		value.Budget.MaxSteps > 0 && value.Used.Steps >= value.Budget.MaxSteps ||
		value.Budget.MaxCostUSD > 0 && value.Used.CostUSD >= value.Budget.MaxCostUSD
}
