package schedules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

var workerTracer = otel.Tracer("lynx/lyra/schedule")

const workerTick = time.Minute

// firingStateWriteTimeout gives an already-attempted firing a bounded window to
// advance its durable cursor after process shutdown cancels the worker context.
// Without this post-attempt scope, a successfully admitted run could be fired
// again on restart because MarkFired observed only the canceled worker context.
const firingStateWriteTimeout = 5 * time.Second

// Runner starts one scheduled prompt as a headless run. It is the
// application-owned seam between a fired schedule and a run start.
type Runner interface {
	StartScheduledRun(ctx context.Context, sc schedule.Schedule) (RunHandle, error)
}

type RunHandle struct {
	SessionID string
	RunID     string
}

// WorkerStore is the schedule persistence slice the worker owns. Management
// CRUD stays on the management use case; the worker only needs the due query
// and guarded cursor advance.
type WorkerStore interface {
	Due(ctx context.Context, now time.Time) ([]schedule.Schedule, error)
	MarkFired(ctx context.Context, id string, ranAt, prevNextRunAt, nextRunAt time.Time) error
}

// Worker scans due schedules and advances their cron cursor after each firing.
// It is the ticker component of the automation use case — the schedule spec and
// next-fire rule are the domain's ([schedule.Schedule] / [schedule.NextRun]);
// the periodic scan and side-effecting firing are the application's.
type Worker struct {
	schedules WorkerStore
	runner    Runner
}

// NewWorker wires a scheduled-run worker.
func NewWorker(schedules WorkerStore, runner Runner) Worker {
	return Worker{schedules: schedules, runner: runner}
}

// Run starts the scheduled-run loop until ctx is canceled.
func (w Worker) Run(ctx context.Context) {
	if w.schedules == nil || w.runner == nil {
		return
	}
	t := time.NewTicker(workerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.fireDue(ctx, time.Now())
		}
	}
}

// Fire starts one schedule through runner under the schedule firing span.
func Fire(ctx context.Context, runner Runner, sc schedule.Schedule) (RunHandle, error) {
	if runner == nil {
		return RunHandle{}, errors.New("schedules: runner is nil")
	}
	ctx, span := workerTracer.Start(ctx, "schedule.fire",
		trace.WithAttributes(attribute.String("schedule.id", sc.ID)))
	defer span.End()
	handle, err := runner.StartScheduledRun(ctx, sc)
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
	due, err := w.schedules.Due(ctx, now)
	if err != nil {
		recordWorkerError(ctx, "due query failed", err)
		return
	}
	for _, sc := range due {
		if ctx.Err() != nil {
			return
		}
		next, nerr := schedule.NextRun(sc.Cron, now)
		if nerr != nil {
			recordWorkerError(ctx, "unparseable cron", fmt.Errorf("schedule %s: %w", sc.ID, nerr))
			next = time.Time{}
		}
		_, fireErr := Fire(ctx, w.runner, sc)
		if fireErr != nil && ctx.Err() != nil && errors.Is(fireErr, ctx.Err()) {
			return
		}
		if fireErr != nil {
			// A rejected run never consumes its occurrence. The durable NextRunAt
			// stays due, so both the next worker tick and a restarted process see
			// the same truthful state. There is deliberately no in-memory retry
			// budget or error classification here.
			recordWorkerError(ctx, "run start failed", fmt.Errorf("schedule %s: %w", sc.ID, fireErr))
			continue
		}

		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), firingStateWriteTimeout)
		markErr := w.schedules.MarkFired(writeCtx, sc.ID, now, sc.NextRunAt, next)
		cancel()
		if markErr != nil {
			recordWorkerError(ctx, "mark fired failed", fmt.Errorf("schedule %s: %w", sc.ID, markErr))
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func recordWorkerError(ctx context.Context, msg string, err error) {
	_, span := workerTracer.Start(ctx, "schedule.error")
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	span.End()
}
