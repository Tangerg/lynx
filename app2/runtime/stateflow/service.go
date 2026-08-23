// Package stateflow owns the remaining Session-scoped Plan and Interrupt read models.
package stateflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/state"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	GetPlan(context.Context, string) (protocol.Plan, error)
	PutPlan(context.Context, protocol.Plan, uint64) error
	ListInterruptSets(context.Context, string, string) ([]protocol.PendingInterruptSet, error)
}

type Service struct {
	store Store
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("stateflow: store is required")
	}
	return &Service{store: store}, nil
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
