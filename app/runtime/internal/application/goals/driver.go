// Package goals owns the autonomous-execution loop (Goal mode): given a
// session's objective, it launches runs back-to-back until the model signals the
// goal complete or blocked (through the update_goal tool), an opt-in cross-turn
// budget is spent, or the user stops it. It mirrors application/schedules — a
// headless application component that drives the runs Coordinator — but is
// event-driven per goal rather than cron-timed, and consumes each run's terminal
// to decide whether to continue.
//
// The loop lives here, NOT in the run pump: the pump holds the session's single
// admission slot across its teardown, so re-entering the coordinator from inside
// it would deadlock. The driver launches the next run only after the previous
// run's stream has fully drained.
package goals

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

var (
	// ErrGoalActive reports a start attempt while the session already has an
	// actively-driving goal.
	ErrGoalActive = errors.New("goals: a goal is already active for this session")
	// ErrNoGoal reports a resume or stop with no goal for the session.
	ErrNoGoal = errors.New("goals: no goal for this session")
)

// RunUseCases is the goal loop's narrow view of the run entry point — the same
// headless start the scheduler uses. Autonomous execution never calls a delivery
// handler.
type RunUseCases interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// Driver owns the per-session autonomous loops. Each active goal has at most one
// loop goroutine, spawned into a task group so shutdown cancels and joins them.
type Driver struct {
	goals goal.Store
	runs  RunUseCases
	tasks *taskgroup.Group
	now   func() time.Time

	mu      sync.Mutex
	running map[string]*loopHandle
}

type loopHandle struct{ cancel context.CancelFunc }

// NewDriver builds the goal driver over the goal store and the run entry point.
// It owns a task group for its loops; call Close at shutdown to cancel them.
func NewDriver(store goal.Store, runUseCases RunUseCases) *Driver {
	return &Driver{
		goals:   store,
		runs:    runUseCases,
		tasks:   &taskgroup.Group{},
		now:     time.Now,
		running: map[string]*loopHandle{},
	}
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving.
func (d *Driver) Start(ctx context.Context, sessionID, objective, provider, model string, budget goal.Budget) (goal.Goal, error) {
	if existing, ok, err := d.goals.Get(ctx, sessionID); err != nil {
		return goal.Goal{}, err
	} else if ok && existing.Status == goal.StatusActive {
		return goal.Goal{}, ErrGoalActive
	}
	g, err := goal.New(sessionID, objective, provider, model, budget, d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	if err := d.goals.Save(ctx, g); err != nil {
		return goal.Goal{}, err
	}
	d.launch(ctx, sessionID)
	return g, nil
}

// Resume returns a paused or blocked goal to active and drives it again. It is
// idempotent on an already-active goal.
func (d *Driver) Resume(ctx context.Context, sessionID string) (goal.Goal, error) {
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if g.Status == goal.StatusActive {
		return g, nil
	}
	g.Resume(d.now())
	if err := d.goals.Save(ctx, g); err != nil {
		return goal.Goal{}, err
	}
	d.launch(ctx, sessionID)
	return g, nil
}

// Stop pauses the session's goal and cancels its loop. The in-flight run (if any)
// finishes on its own; no further run is launched. Returns the goal's state
// before the pause so a caller can report what was stopped.
func (d *Driver) Stop(ctx context.Context, sessionID string) (goal.Goal, error) {
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	d.cancelLoop(sessionID)
	if g.Status == goal.StatusActive {
		g.Pause("stopped by the user", d.now())
		if err := d.goals.Save(ctx, g); err != nil {
			return goal.Goal{}, err
		}
	}
	return g, nil
}

// Get returns the session's goal, or (zero, false, nil) when it has none.
func (d *Driver) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	return d.goals.Get(ctx, sessionID)
}

// Reconcile degrades goals left mid-flight by a previous process. A live loop
// cannot survive a restart, so an active goal becomes paused (resume to continue)
// rather than being silently resumed and left to burn budget; a goal caught at
// the transient complete status is cleared. Run once at startup, before any goal
// can be started.
func (d *Driver) Reconcile(ctx context.Context) error {
	all, err := d.goals.List(ctx)
	if err != nil {
		return err
	}
	for _, g := range all {
		switch g.Status {
		case goal.StatusActive:
			g.Pause("the runtime restarted — resume to continue", d.now())
			if err := d.goals.Save(ctx, g); err != nil {
				return err
			}
		case goal.StatusComplete:
			if err := d.goals.Clear(ctx, g.SessionID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Close cancels and joins every running loop at shutdown.
func (d *Driver) Close() error {
	d.tasks.Close()
	return nil
}

func (d *Driver) cancelLoop(sessionID string) {
	d.mu.Lock()
	if h := d.running[sessionID]; h != nil {
		h.cancel()
		delete(d.running, sessionID)
	}
	d.mu.Unlock()
}
