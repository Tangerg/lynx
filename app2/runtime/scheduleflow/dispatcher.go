package scheduleflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/schedule"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	defaultWorkerTick = 30 * time.Second
	workerBatchSize   = 32
	acceptedRetention = 30 * 24 * time.Hour
	pruneBatchSize    = 256
)

type DispatchStore interface {
	GetSchedule(context.Context, string) (schedule.Schedule, error)
	DueSchedules(context.Context, time.Time, int) ([]schedule.Schedule, error)
	ClaimScheduleOccurrence(context.Context, schedule.Occurrence) (bool, error)
	PendingScheduleOccurrences(context.Context, int) ([]schedule.Occurrence, error)
	PruneAcceptedScheduleOccurrences(context.Context, time.Time, int) error
}

type RunRequest struct {
	Schedule             schedule.Schedule
	OccurrenceID         string
	SessionID            string
	RunID                string
	FiredAt               time.Time
	AllowMissingSchedule bool
}

type StartedRun struct {
	SessionID string
	RunID     string
}

type Runner interface {
	StartScheduledRun(context.Context, RunRequest) (StartedRun, error)
}

type DispatcherConfig struct {
	Store      DispatchStore
	IDs        IDs
	Events     Events
	Runner     Runner
	Lifetime   context.Context
	Clock      func() time.Time
	WorkerTick time.Duration
	Logger     *slog.Logger
}

// Dispatcher owns the active Schedule lifecycle: durable cron claiming,
// crash recovery, and manual firing. Management remains in Service so agents
// and transports do not depend on the Run engine.
type Dispatcher struct {
	store     DispatchStore
	ids       IDs
	events    Events
	runner    Runner
	now       func() time.Time
	tick      time.Duration
	logger    *slog.Logger
	cancel    context.CancelFunc
	tasks     sync.WaitGroup
	closeOnce sync.Once
}

func NewDispatcher(config DispatcherConfig) (*Dispatcher, error) {
	if config.Store == nil || config.IDs == nil || config.Events == nil ||
		config.Runner == nil || config.Lifetime == nil {
		return nil, errors.New("scheduleflow: dispatch store, ids, events, runner, and lifetime are required")
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	tick := config.WorkerTick
	if tick <= 0 {
		tick = defaultWorkerTick
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	lifetime, cancel := context.WithCancel(config.Lifetime)
	dispatcher := &Dispatcher{
		store: config.Store, ids: config.IDs, events: config.Events,
		runner: config.Runner, now: clock, tick: tick, logger: logger, cancel: cancel,
	}
	dispatcher.tasks.Add(1)
	go dispatcher.work(lifetime)
	return dispatcher, nil
}

func (dispatcher *Dispatcher) Close() error {
	if dispatcher == nil {
		return nil
	}
	dispatcher.closeOnce.Do(func() {
		dispatcher.cancel()
		dispatcher.tasks.Wait()
	})
	return nil
}

func (dispatcher *Dispatcher) RunNow(
	ctx context.Context,
	request protocol.RunScheduleNowRequest,
) (*protocol.RunScheduleNowResponse, error) {
	value, err := dispatcher.store.GetSchedule(ctx, request.ID)
	if err != nil {
		return nil, projectError(err)
	}
	sessionID, err := dispatcher.ids.New("ses_")
	if err != nil {
		return nil, err
	}
	runID, err := dispatcher.ids.New("run_")
	if err != nil {
		return nil, err
	}
	started, err := dispatcher.runner.StartScheduledRun(ctx, RunRequest{
		Schedule: value, SessionID: sessionID, RunID: runID, FiredAt: dispatcher.now().UTC(),
	})
	if err != nil {
		return nil, projectError(err)
	}
	dispatcher.publish(value.ID())
	return &protocol.RunScheduleNowResponse{SessionID: started.SessionID, RunID: started.RunID}, nil
}

func (dispatcher *Dispatcher) work(ctx context.Context) {
	defer dispatcher.tasks.Done()
	dispatcher.cycle(ctx)
	ticker := time.NewTicker(dispatcher.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dispatcher.cycle(ctx)
		}
	}
}

func (dispatcher *Dispatcher) cycle(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := dispatcher.store.PruneAcceptedScheduleOccurrences(
		ctx, dispatcher.now().UTC().Add(-acceptedRetention), pruneBatchSize,
	); err != nil {
		dispatcher.logError("prune accepted occurrences", err)
	}
	pending, err := dispatcher.store.PendingScheduleOccurrences(ctx, workerBatchSize)
	if err != nil {
		dispatcher.logError("list pending occurrences", err)
		return
	}
	dispatched := 0
	for _, occurrence := range pending {
		if !dispatcher.dispatch(ctx, occurrence) {
			return
		}
		dispatched++
	}
	remaining := workerBatchSize - dispatched
	if remaining <= 0 {
		return
	}
	now := dispatcher.now().UTC()
	due, err := dispatcher.store.DueSchedules(ctx, now, remaining)
	if err != nil {
		dispatcher.logError("list due schedules", err)
		return
	}
	for _, value := range due {
		if ctx.Err() != nil {
			return
		}
		sessionID, err := dispatcher.ids.New("ses_")
		if err != nil {
			dispatcher.logError("create occurrence Session identity", err)
			continue
		}
		runID, err := dispatcher.ids.New("run_")
		if err != nil {
			dispatcher.logError("create occurrence Run identity", err)
			continue
		}
		occurrence, err := schedule.NewOccurrence(value, now, sessionID, runID)
		if err != nil {
			dispatcher.logError("construct due occurrence", err)
			continue
		}
		claimed, err := dispatcher.store.ClaimScheduleOccurrence(ctx, occurrence)
		if err != nil {
			dispatcher.logError("claim due occurrence", err)
			continue
		}
		if !claimed {
			continue
		}
		dispatcher.publish(value.ID())
		if !dispatcher.dispatch(ctx, occurrence) {
			return
		}
	}
}

func (dispatcher *Dispatcher) dispatch(ctx context.Context, occurrence schedule.Occurrence) bool {
	_, err := dispatcher.runner.StartScheduledRun(ctx, RunRequest{
		Schedule: occurrence.Schedule(), OccurrenceID: occurrence.ID(),
		SessionID: occurrence.SessionID(), RunID: occurrence.RunID(), FiredAt: occurrence.FiredAt(),
		AllowMissingSchedule: true,
	})
	if err == nil {
		dispatcher.publish(occurrence.Schedule().ID())
		return true
	}
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return false
	}
	dispatcher.logError("dispatch schedule occurrence", fmt.Errorf("%s: %w", occurrence.ID(), err))
	return true
}

func (dispatcher *Dispatcher) publish(id string) {
	dispatcher.events.Publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{id},
	})
}

func (dispatcher *Dispatcher) logError(operation string, err error) {
	dispatcher.logger.Error("schedule worker failed", "operation", operation, "error", err)
}
