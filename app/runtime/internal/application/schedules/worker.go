package schedules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

var workerTracer = otel.Tracer("lynx/lyra/schedule")

const workerTick = time.Minute

const manualRunRecordTimeout = 5 * time.Second

// workerBatchSize bounds the durable work admitted by one ticker pass. Pending
// work is oldest-first; newly due schedules are claimed and dispatched together
// so shutdown cannot materialize an unbounded backlog that never reaches Run
// admission.
const workerBatchSize = 32

// Runner starts one scheduled prompt as a headless run. It is the
// application-owned seam between a fired schedule and a run start.
type Runner interface {
	StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (RunHandle, error)
}

type RunHandle struct {
	SessionID string
	RunID     string
}

// WorkerStore is the schedule persistence slice the worker owns. Management
// CRUD stays on the management use case; the worker claims due occurrences and
// re-drives the durable pending work items it previously materialized.
type WorkerStore interface {
	Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error)
	Claim(ctx context.Context, occurrence schedule.Occurrence) (claimed bool, err error)
	Pending(ctx context.Context, limit int) ([]schedule.Occurrence, error)
}

// Worker scans due schedules, atomically materializes occurrence work items,
// and dispatches pending work. It is the ticker component of the automation
// use case — the schedule spec and next-fire rule are the domain's
// ([schedule.Schedule] / [schedule.NextRun]); the periodic scan and side-effecting
// firing are the application's.
type Worker struct {
	schedules WorkerStore
	runner    Runner
	now       func() time.Time
}

// NewWorker wires a scheduled-run worker.
func NewWorker(schedules WorkerStore, runner Runner) Worker {
	return Worker{schedules: schedules, runner: runner, now: time.Now}
}

// Run starts the scheduled-run loop until ctx is canceled.
func (w Worker) Run(ctx context.Context) {
	if w.schedules == nil || w.runner == nil {
		return
	}
	w.fireDue(ctx, w.now())
	t := time.NewTicker(workerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.fireDue(ctx, w.now())
		}
	}
}

// Fire starts one durable schedule occurrence through runner under the schedule
// firing span. An empty occurrence ID denotes a manual run-now, which does not
// consume a cron cursor.
func Fire(ctx context.Context, runner Runner, occurrence schedule.Occurrence) (RunHandle, error) {
	if runner == nil {
		return RunHandle{}, errors.New("schedules: runner is nil")
	}
	ctx, span := workerTracer.Start(ctx, "schedule.fire",
		trace.WithAttributes(attribute.String("schedule.id", occurrence.Schedule.ID)))
	defer span.End()
	handle, err := runner.StartScheduledRun(ctx, occurrence)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		return RunHandle{}, err
	}
	return handle, nil
}

func (w Worker) fireDue(ctx context.Context, now time.Time) {
	if w.schedules == nil || w.runner == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	occurrences, err := w.schedules.Pending(ctx, workerBatchSize)
	if err != nil {
		recordWorkerError(ctx, "pending query failed", err)
		return
	}
	dispatched := 0
	dispatch := func(occurrence schedule.Occurrence) bool {
		if ctx.Err() != nil {
			return false
		}
		dispatched++
		_, fireErr := Fire(ctx, w.runner, occurrence)
		if fireErr != nil && ctx.Err() != nil && errors.Is(fireErr, ctx.Err()) {
			return false
		}
		if fireErr != nil {
			recordWorkerError(ctx, "run start failed", fmt.Errorf("schedule %s: %w", occurrence.Schedule.ID, fireErr))
		}
		return true
	}
	for _, occurrence := range occurrences {
		if dispatched == workerBatchSize || !dispatch(occurrence) {
			return
		}
	}
	if dispatched == workerBatchSize {
		return
	}
	due, err := w.schedules.Due(ctx, now, workerBatchSize-dispatched)
	if err != nil {
		recordWorkerError(ctx, "due query failed", err)
		return
	}
	for _, sc := range due {
		if dispatched == workerBatchSize {
			return
		}
		if ctx.Err() != nil {
			return
		}
		next, nerr := schedule.NextRun(sc.Cron, now)
		if nerr != nil {
			recordWorkerError(ctx, "unparseable cron", fmt.Errorf("schedule %s: %w", sc.ID, nerr))
			next = time.Time{}
		}
		occurrence := schedule.Occurrence{
			ID:        occurrenceID(sc.ID, sc.NextRunAt),
			Schedule:  sc,
			DueAt:     sc.NextRunAt,
			FiredAt:   now.UTC(),
			NextRunAt: next,
			SessionID: "ses_" + uuid.NewString(),
			RunID:     "run_" + uuid.NewString(),
		}
		claimed, claimErr := w.schedules.Claim(ctx, occurrence)
		if claimErr != nil {
			recordWorkerError(ctx, "claim due occurrence failed", fmt.Errorf("schedule %s: %w", sc.ID, claimErr))
			continue
		}
		if claimed {
			if !dispatch(occurrence) {
				return
			}
		}
	}
}

func occurrenceID(scheduleID string, dueAt time.Time) string {
	return scheduleID + ":" + strconv.FormatInt(dueAt.UTC().UnixMilli(), 10)
}

func recordWorkerError(ctx context.Context, msg string, err error) {
	_, span := workerTracer.Start(ctx, "schedule.error")
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	span.End()
}
