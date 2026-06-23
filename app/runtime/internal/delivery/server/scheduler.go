package server

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// The scheduler worker: a background loop, started by `lyra serve`, that fires
// due schedules as headless runs. It is an alternative run ingress — a timer
// instead of a JSON-RPC request — so it lives next to the server that owns the
// persisting run path (StartRun → the pump) rather than in the kernel.

var schedulerTracer = otel.Tracer("lynx/lyra/scheduler")

// schedulerTick is how often the worker scans for due schedules. Cron
// resolution is one minute, so a one-minute tick suffices.
const schedulerTick = time.Minute

// RunScheduler runs the scheduled-run worker until ctx is canceled (the serve
// lifetime). Each tick fires every schedule whose time has come. A nil schedule
// registry (scheduling unconfigured / a test server) makes this a no-op, so the
// caller can start it unconditionally.
func (s *Server) RunScheduler(ctx context.Context) {
	if s.rt.Schedules() == nil {
		return
	}
	t := time.NewTicker(schedulerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.fireDueSchedules(ctx)
		}
	}
}

// fireDueSchedules fires every schedule due as of now and advances each to its
// next future occurrence. A schedule missed during downtime fires once here,
// then jumps to its next slot (MarkFired with NextRun(now)) rather than
// replaying every occurrence it slept through.
func (s *Server) fireDueSchedules(ctx context.Context) {
	now := time.Now()
	due, err := s.rt.Schedules().Due(ctx, now)
	if err != nil {
		recordSchedulerError(ctx, "due query failed", err)
		return
	}
	for _, sc := range due {
		next, nerr := schedule.NextRun(sc.Cron, now)
		if nerr != nil {
			// Cron is validated on write, so a parse failure here is a corrupted
			// record. Zeroing next disables it (rather than re-firing every tick);
			// recorded so the corruption isn't silent.
			recordSchedulerError(ctx, "unparseable cron", fmt.Errorf("schedule %s: %w", sc.ID, nerr))
			next = time.Time{}
		}
		_, _ = s.fireSchedule(ctx, sc) // fire errors are recorded on the per-fire span
		// Advance regardless of fire outcome so a persistently-failing schedule
		// waits for its next slot instead of re-firing every tick (its run lands in
		// the session as a failure). A failed advance leaves it due, so surface it —
		// a silently-swallowed MarkFired error is exactly what turns into a re-fire
		// loop every tick.
		if err := s.rt.Schedules().MarkFired(ctx, sc.ID, now, sc.NextRunAt, next); err != nil {
			recordSchedulerError(ctx, "mark fired failed", fmt.Errorf("schedule %s: %w", sc.ID, err))
		}
	}
}

// recordSchedulerError opens a one-shot span carrying err. The scheduler worker
// fires on a timer with no request/response to surface failures on, so an otel
// span is how a background error that would otherwise be swallowed (a failed
// due-query, an unparseable cron, a failed fired-marker) stays observable.
func recordSchedulerError(ctx context.Context, msg string, err error) {
	_, span := schedulerTracer.Start(ctx, "scheduler.error")
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	span.End()
}

// fireSchedule starts one schedule's saved prompt as a headless run in a fresh
// session and announces it to workspace subscribers. The run drives itself to
// completion and persists independently of any subscriber (the pump runs on a
// detached ctx — openSegment), so this returns once the run is launched. Returns
// the new session id.
func (s *Server) fireSchedule(ctx context.Context, sc schedule.Schedule) (string, error) {
	ctx, span := schedulerTracer.Start(ctx, "scheduler.fire",
		trace.WithAttributes(attribute.String("schedule.id", sc.ID)))
	defer span.End()

	cwd := sc.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	title := sc.Title
	if title == "" {
		title = "Scheduled run"
	}
	sess, err := s.rt.Session().Create(ctx, title, cwd)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create session")
		return "", err
	}

	// Fire-and-forget: a child ctx canceled right after StartRun drops our
	// (unused) event subscription immediately; the run keeps going on its own
	// detached ctx and the pump persists it without a subscriber.
	fireCtx, cancel := context.WithCancel(ctx)
	_, _, err = s.StartRun(fireCtx, protocol.StartRunRequest{
		SessionID: sess.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: sc.Prompt}},
		Provider:  sc.Provider,
		Model:     sc.Model,
	})
	cancel()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start run")
		return "", err
	}

	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
		Type:       protocol.WorkspaceEventSchedulesFired,
		ScheduleID: sc.ID,
	})
	return sess.ID, nil
}
