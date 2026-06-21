package server

import (
	"context"
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
		_, span := schedulerTracer.Start(ctx, "scheduler.tick")
		span.RecordError(err)
		span.SetStatus(codes.Error, "due query failed")
		span.End()
		return
	}
	for _, sc := range due {
		next, nerr := schedule.NextRun(sc.Cron, now)
		if nerr != nil {
			next = time.Time{} // unparseable cron (validated on write) → stop firing it
		}
		_, _ = s.fireSchedule(ctx, sc) // errors recorded on the per-fire span
		// Advance regardless of fire outcome so a persistently-failing schedule
		// doesn't re-fire every tick (its run lands in the session as a failure).
		_ = s.rt.Schedules().MarkFired(ctx, sc.ID, now, next)
	}
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
