package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var workerTracer = otel.Tracer("lynx/lyra/schedule")

const workerTick = time.Minute

// Runner starts one scheduled prompt as a headless run.
type Runner interface {
	StartScheduledRun(ctx context.Context, sc Schedule) (string, error)
}

// Worker scans due schedules and advances their cron cursor after each firing.
type Worker struct {
	schedules Service
	runner    Runner
}

// NewWorker wires a scheduled-run worker.
func NewWorker(schedules Service, runner Runner) Worker {
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
func Fire(ctx context.Context, runner Runner, sc Schedule) (string, error) {
	if runner == nil {
		return "", errors.New("schedule: runner is nil")
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
		next, nerr := NextRun(sc.Cron, now)
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
