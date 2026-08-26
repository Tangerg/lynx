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

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
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

// ScheduledRunStarter starts one scheduled instruction set as a headless run. It is the
// application-owned seam between a fired schedule and a run start.
type ScheduledRunStarter interface {
	StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (StartedRun, error)
}

// StartedRun identifies the Run accepted for one schedule occurrence.
type StartedRun struct {
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
	schedules     WorkerStore
	runStarter    ScheduledRunStarter
	now           func() time.Time
	invalidations invalidation.Publish
}

// NewWorker wires a scheduled-run worker.
func NewWorker(schedules WorkerStore, runStarter ScheduledRunStarter, invalidations invalidation.Publish) Worker {
	return Worker{schedules: schedules, runStarter: runStarter, now: time.Now, invalidations: invalidations}
}

// Run starts the scheduled-run loop until ctx is canceled.
func (w Worker) Run(ctx context.Context) {
	if w.schedules == nil || w.runStarter == nil {
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
func Fire(ctx context.Context, runStarter ScheduledRunStarter, occurrence schedule.Occurrence) (StartedRun, error) {
	if runStarter == nil {
		return StartedRun{}, errors.New("schedules: scheduled run starter is nil")
	}
	ctx, span := workerTracer.Start(ctx, "schedule.fire",
		trace.WithAttributes(attribute.String("schedule.id", occurrence.Schedule.ID)))
	defer span.End()
	handle, err := runStarter.StartScheduledRun(ctx, occurrence)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		return StartedRun{}, err
	}
	return handle, nil
}

func (w Worker) fireDue(ctx context.Context, now time.Time) {
	if w.schedules == nil || w.runStarter == nil {
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
	batch := occurrenceBatch{ctx: ctx, runStarter: w.runStarter, invalidations: w.invalidations}
	if !batch.dispatchAll(occurrences) || batch.full() {
		return
	}
	due, err := w.schedules.Due(ctx, now, batch.remaining())
	if err != nil {
		recordWorkerError(ctx, "due query failed", err)
		return
	}
	for _, scheduled := range due {
		if batch.full() {
			return
		}
		if ctx.Err() != nil {
			return
		}
		occurrence, claimed := w.claimDueOccurrence(ctx, scheduled, now)
		if claimed && !batch.dispatch(occurrence) {
			return
		}
	}
}

type occurrenceBatch struct {
	ctx           context.Context
	runStarter    ScheduledRunStarter
	dispatched    int
	invalidations invalidation.Publish
}

func (o *occurrenceBatch) remaining() int { return workerBatchSize - o.dispatched }

func (o *occurrenceBatch) full() bool { return o.remaining() == 0 }

func (o *occurrenceBatch) dispatchAll(occurrences []schedule.Occurrence) bool {
	for _, occurrence := range occurrences {
		if o.full() || !o.dispatch(occurrence) {
			return false
		}
	}
	return true
}

func (o *occurrenceBatch) dispatch(occurrence schedule.Occurrence) bool {
	if o.ctx.Err() != nil {
		return false
	}
	o.dispatched++
	_, err := Fire(o.ctx, o.runStarter, occurrence)
	if err == nil {
		o.invalidations.Notify(invalidation.ForSchedules(occurrence.Schedule.ID))
	}
	if err != nil && o.ctx.Err() != nil && errors.Is(err, o.ctx.Err()) {
		return false
	}
	if err != nil {
		recordWorkerError(
			o.ctx,
			"run start failed",
			fmt.Errorf("schedule %s: %w", occurrence.Schedule.ID, err),
		)
	}
	return true
}

func (w Worker) claimDueOccurrence(
	ctx context.Context,
	scheduled schedule.Schedule,
	now time.Time,
) (schedule.Occurrence, bool) {
	nextRunAt, err := schedule.NextRun(scheduled.Cron, now)
	if err != nil {
		recordWorkerError(ctx, "unparseable cron", fmt.Errorf("schedule %s: %w", scheduled.ID, err))
		nextRunAt = time.Time{}
	}
	occurrence := schedule.Occurrence{
		ID:        occurrenceID(scheduled.ID, scheduled.NextRunAt),
		Schedule:  scheduled,
		DueAt:     scheduled.NextRunAt,
		FiredAt:   now.UTC(),
		NextRunAt: nextRunAt,
		SessionID: "ses_" + uuid.NewString(),
		RunID:     "run_" + uuid.NewString(),
	}
	claimed, err := w.schedules.Claim(ctx, occurrence)
	if err != nil {
		recordWorkerError(
			ctx,
			"claim due occurrence failed",
			fmt.Errorf("schedule %s: %w", scheduled.ID, err),
		)
		return schedule.Occurrence{}, false
	}
	if claimed {
		// Claim advances NextRunAt before Run admission. Publish that committed
		// cursor even when the following start fails; a later pending retry that is
		// accepted publishes again for LastRunAt.
		w.invalidations.Notify(invalidation.ForSchedules(scheduled.ID))
	}
	return occurrence, claimed
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
