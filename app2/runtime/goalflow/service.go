// Package goalflow owns autonomous Goal lifecycle and durable Run ownership.
// The Service performs commands; Driver is the only component that schedules
// work. Both coordinate through exact Goal incarnation/revision CAS.
package goalflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	SessionExists(context.Context, string) (bool, error)
	LoadGoal(context.Context, string) (goaldomain.Goal, error)
	LoadGoalByRun(context.Context, string) (goaldomain.Goal, error)
	ListGoals(context.Context) ([]goaldomain.Goal, error)
	CreateGoal(context.Context, goaldomain.Goal) error
	SaveGoal(context.Context, goaldomain.Goal, string, uint64) (goaldomain.Goal, error)
	RemoveGoal(context.Context, string, string, uint64) error
}

type IDs interface { New(string) (string, error) }

type Publisher interface { Publish(protocol.RuntimeEvent) }

type Service struct {
	store   Store
	ids     IDs
	signals *Signals
	events  Publisher
	now     func() time.Time
}

func New(store Store, ids IDs, signals *Signals, events Publisher) (*Service, error) {
	if store == nil || ids == nil || signals == nil || events == nil { return nil, errors.New("goalflow: store, ids, signals and event publisher are required") }
	return &Service{store: store, ids: ids, signals: signals, events: events, now: time.Now}, nil
}

func (service *Service) Start(ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
	if strings.TrimSpace(request.Objective) == "" || (request.Provider == "") != (request.Model == "") {
		return nil, fmt.Errorf("%w: invalid goal", protocol.ErrInvalidParams)
	}
	budget := goaldomain.Budget{MaxRuns: request.Budget.MaxRuns, MaxCostUSD: request.Budget.MaxCostUSD, MaxSteps: request.Budget.MaxSteps}
	if err := budget.Validate(); err != nil { return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err) }
	exists, err := service.store.SessionExists(ctx, request.SessionID)
	if err != nil { return nil, err }
	if !exists { return nil, protocol.ErrSessionNotFound }
	if _, err := service.store.LoadGoal(ctx, request.SessionID); err == nil {
		return nil, protocol.ErrSessionBusy
	} else if !errors.Is(err, goaldomain.ErrNotFound) { return nil, err }
	incarnation, err := service.ids.New("goal_")
	if err != nil { return nil, err }
	value, err := goaldomain.New(goaldomain.Create{
		SessionID: request.SessionID, IncarnationID: incarnation, Objective: request.Objective,
		Provider: request.Provider, Model: request.Model, Budget: budget, Now: service.now(),
	})
	if err != nil { return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err) }
	if err := service.store.CreateGoal(ctx, value); err != nil {
		if errors.Is(err, goaldomain.ErrVersionConflict) { return nil, protocol.ErrSessionBusy }
		return nil, err
	}
	service.changed(request.SessionID)
	return Present(value), nil
}

func (service *Service) Update(ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
	if strings.TrimSpace(request.Objective) == "" { return nil, fmt.Errorf("%w: objective is required", protocol.ErrInvalidParams) }
	value, err := service.load(ctx, request.SessionID)
	if err != nil { return nil, err }
	incarnation, err := service.ids.New("goal_")
	if err != nil { return nil, err }
	expectedIncarnation, expectedRevision := value.IncarnationID(), value.Revision()
	if err := value.ReplaceObjective(request.Objective, incarnation, service.now()); err != nil { return nil, fmt.Errorf("%w: goal cannot be edited", protocol.ErrInvalidParams) }
	value, err = service.store.SaveGoal(ctx, value, expectedIncarnation, expectedRevision)
	if err != nil { return nil, service.project(err) }
	service.changed(request.SessionID)
	return Present(value), nil
}

func (service *Service) Get(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, err := service.store.LoadGoal(ctx, request.SessionID)
	if errors.Is(err, goaldomain.ErrNotFound) { return nil, nil }
	if err != nil { return nil, err }
	return Present(value), nil
}

func (service *Service) Stop(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, err := service.load(ctx, request.SessionID)
	if err != nil { return nil, err }
	expectedIncarnation, expectedRevision := value.IncarnationID(), value.Revision()
	if err := value.Pause(goaldomain.ReasonStoppedByUser, "", service.now()); err != nil { return nil, fmt.Errorf("%w: goal is not active", protocol.ErrInvalidParams) }
	value, err = service.store.SaveGoal(ctx, value, expectedIncarnation, expectedRevision)
	if err != nil { return nil, service.project(err) }
	service.changed(request.SessionID)
	return Present(value), nil
}

func (service *Service) Resume(ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
	value, err := service.load(ctx, request.SessionID)
	if err != nil { return nil, err }
	expectedIncarnation, expectedRevision := value.IncarnationID(), value.Revision()
	if err := value.Resume(service.now()); err != nil { return nil, fmt.Errorf("%w: goal is not resumable or its budget is exhausted", protocol.ErrInvalidParams) }
	value, err = service.store.SaveGoal(ctx, value, expectedIncarnation, expectedRevision)
	if err != nil { return nil, service.project(err) }
	service.changed(request.SessionID)
	return Present(value), nil
}

func (service *Service) Clear(ctx context.Context, request protocol.GoalRequest) error {
	value, err := service.load(ctx, request.SessionID)
	if err != nil { return err }
	if err := service.store.RemoveGoal(ctx, value.SessionID(), value.IncarnationID(), value.Revision()); err != nil { return service.project(err) }
	service.changed(request.SessionID)
	return nil
}

func (service *Service) Current(ctx context.Context, sessionID string) (goaldomain.Goal, bool, error) {
	value, err := service.store.LoadGoal(ctx, sessionID)
	if errors.Is(err, goaldomain.ErrNotFound) { return goaldomain.Goal{}, false, nil }
	return value, err == nil, err
}

func (service *Service) IsOwnedRun(ctx context.Context, sessionID, runID string) (bool, error) {
	value, found, err := service.Current(ctx, sessionID)
	return found && value.ActiveRunID() == runID, err
}

func (service *Service) continueRun(ctx context.Context, runID string) (goaldomain.Goal, error) {
	value, err := service.store.LoadGoalByRun(ctx, runID)
	if err != nil { return goaldomain.Goal{}, err }
	expectedRevision := value.Revision()
	if err := value.ContinueRun(runID, service.now()); err != nil { return goaldomain.Goal{}, err }
	stored, err := service.store.SaveGoal(ctx, value, value.IncarnationID(), expectedRevision)
	if err == nil { service.changed(value.SessionID()) }
	return stored, err
}

func (service *Service) Report(ctx context.Context, sessionID, runID, outcome, reason string) (string, error) {
	value, err := service.store.LoadGoal(ctx, sessionID)
	if errors.Is(err, goaldomain.ErrNotFound) { return "No active Goal exists for this session.", nil }
	if err != nil { return "", err }
	if value.ActiveRunID() != runID { return "This Run does not own the current Goal; no outcome was reported.", nil }
	completed := outcome == "completed"
	if !completed && outcome != "blocked" { return "Invalid Goal outcome; use completed or blocked.", nil }
	expectedIncarnation, expectedRevision := value.IncarnationID(), value.Revision()
	if err := value.Report(expectedIncarnation, completed, reason, service.now()); err != nil {
		if outcome == "blocked" && strings.TrimSpace(reason) == "" { return "Provide a concrete reason when reporting a blocked Goal.", nil }
		return "The Goal is no longer active; no outcome was reported.", nil
	}
	if _, err := service.store.SaveGoal(ctx, value, expectedIncarnation, expectedRevision); err != nil {
		if errors.Is(err, goaldomain.ErrVersionConflict) { return "The Goal changed concurrently; inspect it before reporting an outcome.", nil }
		return "", err
	}
	service.changed(sessionID)
	if completed { return "Goal outcome reported as completed. The loop will stop after this Run settles.", nil }
	return "Goal outcome reported as blocked. The loop will stop and surface the reason.", nil
}

func (service *Service) claimRun(ctx context.Context, sessionID, incarnationID, runID string) (goaldomain.Goal, error) {
	value, err := service.store.LoadGoal(ctx, sessionID)
	if err != nil { return goaldomain.Goal{}, err }
	if value.IncarnationID() != incarnationID { return goaldomain.Goal{}, goaldomain.ErrVersionConflict }
	expectedRevision := value.Revision()
	if err := value.ClaimRun(runID, service.now()); err != nil { return goaldomain.Goal{}, err }
	stored, err := service.store.SaveGoal(ctx, value, incarnationID, expectedRevision)
	if err == nil { service.changed(sessionID) }
	return stored, err
}

func (service *Service) awaitInput(ctx context.Context, value goaldomain.Goal) (goaldomain.Goal, error) {
	expectedRevision := value.Revision()
	if err := value.AwaitInput(value.ActiveRunID(), service.now()); err != nil { return goaldomain.Goal{}, err }
	stored, err := service.store.SaveGoal(ctx, value, value.IncarnationID(), expectedRevision)
	if err == nil { service.changed(value.SessionID()) }
	return stored, err
}

func (service *Service) pause(ctx context.Context, value goaldomain.Goal, code goaldomain.ReasonCode, detail string) (goaldomain.Goal, error) {
	expectedRevision := value.Revision()
	if err := value.Pause(code, detail, service.now()); err != nil { return goaldomain.Goal{}, err }
	stored, err := service.store.SaveGoal(ctx, value, value.IncarnationID(), expectedRevision)
	if err == nil { service.changed(value.SessionID()) }
	return stored, err
}

func (service *Service) settleRun(ctx context.Context, value goaldomain.Goal, runID, outcome string, usage protocol.RunMetrics) (bool, error) {
	cost := 0.0
	if usage.Usage != nil && usage.Usage.CostUSD != nil { cost = *usage.Usage.CostUSD }
	expectedRevision := value.Revision()
	remove, err := value.SettleRun(goaldomain.RunSettlement{RunID: runID, Outcome: outcome, CostUSD: cost, Steps: usage.Steps, Now: service.now()})
	if err != nil { return false, err }
	if remove {
		if err := service.store.RemoveGoal(ctx, value.SessionID(), value.IncarnationID(), expectedRevision); err != nil { return false, err }
		service.changed(value.SessionID())
		return true, nil
	}
	if _, err := service.store.SaveGoal(ctx, value, value.IncarnationID(), expectedRevision); err != nil { return false, err }
	service.changed(value.SessionID())
	return false, nil
}

func (service *Service) recoverWithoutRun(ctx context.Context, value goaldomain.Goal) (goaldomain.Goal, error) {
	expectedRevision := value.Revision()
	if err := value.RecoverWithoutRun(service.now()); err != nil { return goaldomain.Goal{}, err }
	stored, err := service.store.SaveGoal(ctx, value, value.IncarnationID(), expectedRevision)
	if err == nil { service.changed(value.SessionID()) }
	return stored, err
}

func (service *Service) changed(sessionID string) {
	service.signals.Publish(sessionID)
	service.events.Publish(protocol.RuntimeEvent{Type: protocol.RuntimeGoalsChanged, SessionIDs: []string{sessionID}})
}

func (service *Service) load(ctx context.Context, sessionID string) (goaldomain.Goal, error) {
	value, err := service.store.LoadGoal(ctx, sessionID)
	if errors.Is(err, goaldomain.ErrNotFound) { return goaldomain.Goal{}, fmt.Errorf("%w: no goal for session", protocol.ErrInvalidParams) }
	return value, err
}

func (service *Service) project(err error) error {
	if errors.Is(err, goaldomain.ErrVersionConflict) { return protocol.ErrRevisionConflict }
	if errors.Is(err, goaldomain.ErrNotFound) { return fmt.Errorf("%w: no goal for session", protocol.ErrInvalidParams) }
	return err
}

func Present(value goaldomain.Goal) *protocol.Goal {
	budget := value.Budget(); used := value.Used(); reason := value.Reason()
	result := &protocol.Goal{
		SessionID: value.SessionID(), Objective: value.Objective(), Status: protocol.GoalStatus(value.Status()),
		Provider: value.Provider(), Model: value.Model(),
		Budget: protocol.GoalBudget{MaxRuns: budget.MaxRuns, MaxCostUSD: budget.MaxCostUSD, MaxSteps: budget.MaxSteps},
		Used: protocol.GoalUsage{Runs: used.Runs, CostUSD: used.CostUSD, Steps: used.Steps},
		CreatedAt: value.CreatedAt(), UpdatedAt: value.UpdatedAt(),
	}
	if reason.Code != goaldomain.ReasonNone { result.Reason = &protocol.GoalReason{Code: protocol.GoalReasonCode(reason.Code), Detail: reason.Detail} }
	return result
}
