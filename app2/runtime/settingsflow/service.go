// Package settingsflow owns scheduled-run resources and their worker.
package settingsflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Tangerg/lynx/app2/runtime/domain/settings"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	ListSchedules(context.Context) ([]protocol.Schedule, error)
	GetSchedule(context.Context, string) (protocol.Schedule, error)
	PutSchedule(context.Context, protocol.Schedule, *uint64) error
	DeleteSchedule(context.Context, string) error
}

type IDs interface{ New(string) (string, error) }

type ScheduleRunner interface {
	RunSchedule(context.Context, protocol.Schedule) (string, string, error)
}

type Service struct {
	store Store
	ids IDs
	runner ScheduleRunner
	now func() time.Time
	cancel context.CancelFunc
	tasks sync.WaitGroup
	closeOnce sync.Once
}

// Start owns the schedule worker for the supplied Runtime lifetime. Claiming a
// due row advances its revision before launching, so concurrent Runtime
// processes cannot both fire the same occurrence.
func (service *Service) Start(lifetime context.Context) error {
	if lifetime == nil {
		return errors.New("settingsflow: lifetime is required")
	}
	ctx, cancel := context.WithCancel(lifetime)
	service.cancel = cancel
	service.tasks.Add(1)
	go func() {
		defer service.tasks.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		service.fireDue(ctx)
		for {
			select {
			case <-ticker.C:
				service.fireDue(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (service *Service) Close() {
	service.closeOnce.Do(func() {
		if service.cancel != nil {
			service.cancel()
		}
		service.tasks.Wait()
	})
}

func New(store Store, ids IDs, runner ScheduleRunner) (*Service, error) {
	if store == nil || ids == nil {
		return nil, errors.New("settingsflow: store and ids are required")
	}
	return &Service{store: store, ids: ids, runner: runner, now: time.Now}, nil
}

func (service *Service) ListSchedules(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
	values, err := service.store.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(values), nil
}

func (service *Service) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	if strings.TrimSpace(request.Instructions) == "" {
		return nil, fmt.Errorf("%w: instructions are required", protocol.ErrInvalidParams)
	}
	next, err := nextRun(request.Cron, service.now())
	if err != nil {
		return nil, err
	}
	id, err := service.ids.New("sch_")
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = "Scheduled task"
	}
	now := service.now().UTC()
	value := protocol.Schedule{
		ID: id, Title: title, Instructions: request.Instructions, Workspace: request.Workspace,
		Provider: request.Provider, Model: request.Model, Cron: request.Cron, Enabled: true,
		NextRunAt: &next, CreatedAt: now, Revision: 1,
	}
	if err := service.store.PutSchedule(ctx, value, nil); err != nil {
		return nil, err
	}
	return &value, nil
}

func (service *Service) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	value, err := service.store.GetSchedule(ctx, request.ID)
	if err != nil {
		return nil, service.scheduleError(err)
	}
	if value.Revision != request.ExpectedRevision {
		return nil, protocol.ErrRevisionConflict
	}
	if request.Title != nil {
		value.Title = strings.TrimSpace(*request.Title)
	}
	if request.Instructions != nil {
		value.Instructions = *request.Instructions
	}
	if request.Workspace != nil {
		value.Workspace = request.Workspace
	}
	if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault {
		value.Workspace = nil
	}
	if request.Provider != nil {
		value.Provider = *request.Provider
	}
	if request.Model != nil {
		value.Model = *request.Model
	}
	if request.Cron != nil {
		value.Cron = *request.Cron
	}
	if request.Enabled != nil {
		value.Enabled = *request.Enabled
	}
	if value.Enabled {
		next, err := nextRun(value.Cron, service.now())
		if err != nil {
			return nil, err
		}
		value.NextRunAt = &next
	} else {
		value.NextRunAt = nil
	}
	previous := value.Revision
	value.Revision++
	if err := service.store.PutSchedule(ctx, value, &previous); err != nil {
		return nil, err
	}
	return &value, nil
}

func (service *Service) DeleteSchedule(ctx context.Context, request protocol.DeleteScheduleRequest) error {
	if err := service.store.DeleteSchedule(ctx, request.ID); err != nil {
		return service.scheduleError(err)
	}
	return nil
}

func (service *Service) RunNow(ctx context.Context, request protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
	value, err := service.store.GetSchedule(ctx, request.ID)
	if err != nil {
		return nil, service.scheduleError(err)
	}
	if service.runner == nil {
		return nil, errors.New("settingsflow: schedule runner is unavailable")
	}
	sessionID, runID, err := service.runner.RunSchedule(ctx, value)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	value.LastRunAt = &now
	if value.Enabled {
		next, nextErr := nextRun(value.Cron, now)
		if nextErr != nil {
			return nil, nextErr
		}
		value.NextRunAt = &next
	}
	previous := value.Revision
	value.Revision++
	if err := service.store.PutSchedule(ctx, value, &previous); err != nil {
		return nil, err
	}
	return &protocol.RunScheduleNowResponse{SessionID: sessionID, RunID: runID}, nil
}

func (service *Service) scheduleError(err error) error {
	if errors.Is(err, settings.ErrNotFound) {
		return protocol.ErrItemNotFound
	}
	return err
}

func (service *Service) fireDue(ctx context.Context) {
	if service.runner == nil {
		return
	}
	values, err := service.store.ListSchedules(ctx)
	if err != nil {
		return
	}
	now := service.now().UTC()
	for _, value := range values {
		if !value.Enabled || value.NextRunAt == nil || value.NextRunAt.After(now) {
			continue
		}
		next, err := nextRun(value.Cron, now)
		if err != nil {
			continue
		}
		previous := value.Revision
		value.Revision++
		value.LastRunAt = &now
		value.NextRunAt = &next
		if err := service.store.PutSchedule(ctx, value, &previous); err != nil {
			continue
		}
		_, _, _ = service.runner.RunSchedule(ctx, value)
	}
}

func nextRun(expression string, after time.Time) (time.Time, error) {
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: cron: %v", protocol.ErrInvalidParams, err)
	}
	return schedule.Next(after).UTC(), nil
}
