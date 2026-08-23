// Package planflow owns Plan replacement, Plan-mode policy, and protocol
// projection. The ordered Plan aggregate remains independent from tools and IO.
package planflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	LoadPlan(context.Context, string) (plandomain.State, error)
	SavePlan(context.Context, plandomain.State, uint64) error
	EnterPlanMode(context.Context, string, time.Time) (bool, error)
	ExitPlanMode(context.Context, string) (bool, error)
	IsPlanMode(context.Context, string) (bool, error)
}

type Publisher interface { Publish(protocol.RuntimeEvent) }

type Service struct {
	store  Store
	events Publisher
	now    func() time.Time
}

func New(store Store, events Publisher) (*Service, error) {
	if store == nil || events == nil { return nil, errors.New("planflow: store and event publisher are required") }
	return &Service{store: store, events: events, now: time.Now}, nil
}

func (service *Service) Get(ctx context.Context, request protocol.GetPlanRequest) (*protocol.Plan, error) {
	value, err := service.store.LoadPlan(ctx, request.SessionID)
	if errors.Is(err, plandomain.ErrNotFound) { return nil, protocol.ErrSessionNotFound }
	if err != nil { return nil, err }
	return Present(value), nil
}

func (service *Service) Replace(ctx context.Context, sessionID string, steps []protocol.PlanStep) (*protocol.Plan, error) {
	current, err := service.store.LoadPlan(ctx, sessionID)
	if errors.Is(err, plandomain.ErrNotFound) { return nil, protocol.ErrSessionNotFound }
	if err != nil { return nil, err }
	domainSteps := make([]plandomain.Step, len(steps))
	for index, step := range steps { domainSteps[index] = plandomain.Step{Description: step.Description, Status: plandomain.Status(step.Status)} }
	next, err := current.Replace(domainSteps, service.now())
	if err != nil { return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err) }
	if err := service.store.SavePlan(ctx, next, current.Revision()); err != nil {
		if errors.Is(err, plandomain.ErrVersionConflict) { return nil, protocol.ErrRevisionConflict }
		return nil, err
	}
	service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimePlanChanged, SessionIDs: []string{sessionID}})
	return Present(next), nil
}

func (service *Service) EnterMode(ctx context.Context, sessionID string) (bool, error) {
	if _, err := service.store.LoadPlan(ctx, sessionID); err != nil {
		if errors.Is(err, plandomain.ErrNotFound) { return false, protocol.ErrSessionNotFound }
		return false, err
	}
	return service.store.EnterPlanMode(ctx, sessionID, service.now())
}

func (service *Service) ExitMode(ctx context.Context, sessionID string) (bool, error) {
	return service.store.ExitPlanMode(ctx, sessionID)
}

func (service *Service) Mode(ctx context.Context, sessionID string) (bool, error) {
	return service.store.IsPlanMode(ctx, sessionID)
}

func Present(value plandomain.State) *protocol.Plan {
	steps := value.Steps()
	presented := make([]protocol.PlanStep, len(steps))
	for index, step := range steps {
		presented[index] = protocol.PlanStep{ID: strconv.Itoa(index), Description: step.Description, Status: protocol.PlanStatus(step.Status)}
	}
	return &protocol.Plan{SessionID: value.SessionID(), Revision: value.Revision(), Steps: presented, UpdatedAt: value.UpdatedAt()}
}
