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

// Runner starts one scheduled prompt as a headless run. The delivery layer
// supplies it; it is the application-owned seam between a fired schedule and a
// run start.
type Runner interface {
	StartScheduledRun(ctx context.Context, sc schedule.Schedule) (string, error)
}

// WorkerStore is the schedule persistence slice the worker owns. Management CRUD
// stays on [schedule.Registry]; the worker only needs the due query and guarded
// cursor advance.
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
func Fire(ctx context.Context, runner Runner, sc schedule.Schedule) (string, error) {
	if runner == nil {
		return "", errors.New("schedules: runner is nil")
	}
	ctx, span := workerTracer.Start(ctx, "schedule.fire",
		trace.WithAttributes(attribute.String("schedule.id", sc.ID)))
	defer span.End()
	sessionID, err := runner.StartScheduledRun(ctx, sc)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		return "", err
	}
	return sessionID, nil
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
		next, nerr := schedule.NextRun(sc.Cron, now)
		if nerr != nil {
			recordWorkerError(ctx, "unparseable cron", fmt.Errorf("schedule %s: %w", sc.ID, nerr))
			next = time.Time{}
		}
		_, _ = Fire(ctx, w.runner, sc)
		if err := w.schedules.MarkFired(ctx, sc.ID, now, sc.NextRunAt, next); err != nil {
			recordWorkerError(ctx, "mark fired failed", fmt.Errorf("schedule %s: %w", sc.ID, err))
		}
	}
}

func recordWorkerError(ctx context.Context, msg string, err error) {
	_, span := workerTracer.Start(ctx, "schedule.error")
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	span.End()
}
