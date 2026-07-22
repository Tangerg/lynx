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
	// ErrNoSession reports a start for a session that does not exist — a goal is
	// session-owned, so it must never outlive (or precede) its session.
	ErrNoSession = errors.New("goals: session does not exist")
	// ErrGoalConflict reports that a concurrent lifecycle transition won the goal's
	// compare-and-swap; the caller read a generation that was already superseded.
	ErrGoalConflict = errors.New("goals: goal changed concurrently")
)

// RunUseCases is the goal loop's narrow view of the run entry point — the same
// headless start the scheduler uses. Autonomous execution never calls a delivery
// handler.
type RunUseCases interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// SessionExists reports whether a session id refers to a live session. The
// driver refuses to open a goal for a session that does not exist (no orphan
// goals), and the boot reconcile clears goals whose session was deleted while
// the runtime was down.
type SessionExists interface {
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// Driver owns the per-session autonomous loops. Each active goal has at most one
// loop goroutine, spawned into a task group so shutdown cancels and joins them.
//
// Every durable goal write is a compare-and-swap on the goal's generation
// ([goal.Store]); the mutex here guards only the in-process loop registry, not
// the durable state. Explicit lifecycle transitions (Start/Resume/Stop) advance
// the generation, so a canceled loop's request-detached terminal write is
// rejected by the store instead of overwriting a newer goal or resurrecting a
// cleared one — the correctness guarantee that does NOT require blocking on the
// old loop's exit (which could stall a delete/stop on a whole model turn).
type Driver struct {
	goals    goal.Store
	runs     RunUseCases
	sessions SessionExists
	tasks    *taskgroup.Group
	now      func() time.Time

	// cmdMu serializes the lifecycle commands (Start / Resume / Stop) so their
	// read → decide → write → launch/cancel sequences never interleave. Without
	// it a Stop could cancel a loop a concurrent Resume just launched and then
	// skip its own durable write — it decides on a status read taken before the
	// cancel — wedging the goal into active-with-no-loop. The loop goroutines and
	// update_goal deliberately do NOT take this lock; their races are handled by
	// the store's generation CAS, not by serializing against user commands.
	cmdMu sync.Mutex

	mu      sync.Mutex // guards running
	running map[string]*loopHandle
}

type loopHandle struct{ cancel context.CancelFunc }

// NewDriver builds the goal driver over the goal store, the run entry point, and
// the session-existence check. It owns a task group for its loops; call Close at
// shutdown to cancel them.
func NewDriver(store goal.Store, runUseCases RunUseCases, sessions SessionExists) *Driver {
	return &Driver{
		goals:    store,
		runs:     runUseCases,
		sessions: sessions,
		tasks:    &taskgroup.Group{},
		now:      time.Now,
		running:  map[string]*loopHandle{},
	}
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving, and refuses a session that
// does not exist. The new goal advances the generation so any straggler loop
// from the replaced goal can no longer write.
func (d *Driver) Start(ctx context.Context, sessionID, objective, provider, model string, budget goal.Budget) (goal.Goal, error) {
	d.cmdMu.Lock()
	defer d.cmdMu.Unlock()
	exists, err := d.sessions.Exists(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !exists {
		return goal.Goal{}, ErrNoSession
	}
	existing, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if ok && existing.Status == goal.StatusActive {
		return goal.Goal{}, ErrGoalActive
	}
	var expected int64
	if ok {
		expected = existing.Generation
	}
	g, err := goal.New(sessionID, objective, provider, model, budget, d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	g.Generation = expected + 1
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	d.launch(ctx, sessionID, g.Generation)
	return g, nil
}

// Resume returns a paused or blocked goal to active and drives it again. It is
// idempotent on an already-active goal. The resume advances the generation so
// the fresh loop owns the goal and any straggler cannot write.
func (d *Driver) Resume(ctx context.Context, sessionID string) (goal.Goal, error) {
	d.cmdMu.Lock()
	defer d.cmdMu.Unlock()
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
	expected := g.Generation
	g.Resume(d.now())
	g.Generation = expected + 1
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	d.launch(ctx, sessionID, g.Generation)
	return g, nil
}

// Stop pauses the session's goal and cancels its loop. Canceling is enough: the
// generation bump makes the paused state authoritative, so the straggler loop's
// detached write is rejected — Stop need not block on the old loop's exit (its
// in-flight run finishes on its own; no further run is launched). Returns the
// goal's current state so a caller can report what was stopped.
func (d *Driver) Stop(ctx context.Context, sessionID string) (goal.Goal, error) {
	d.cmdMu.Lock()
	defer d.cmdMu.Unlock()
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	d.Quiesce(sessionID)
	if g.Status != goal.StatusActive {
		return g, nil
	}
	expected := g.Generation
	g.Pause("stopped by the user", d.now())
	g.Generation = expected + 1
	if _, err := d.goals.Save(ctx, g, expected); err != nil {
		return goal.Goal{}, err
	}
	// Report the authoritative stored state: our pause if the CAS won, else what a
	// concurrent transition (a double-stop, or the loop's own terminal write)
	// committed. If it was cleared out from under us, report the pause we intended.
	current, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return g, nil
	}
	return current, nil
}

// Get returns the session's goal, or (zero, false, nil) when it has none.
func (d *Driver) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	return d.goals.Get(ctx, sessionID)
}

// Reconcile degrades goals left mid-flight by a previous process. A goal whose
// session no longer exists (deleted while the runtime was down) is cleared — the
// orphan sweep. A live loop cannot survive a restart, so an active goal becomes
// paused (resume to continue) rather than being silently resumed and left to
// burn budget; a goal caught at the transient complete status is cleared. Run
// once at startup, before any goal can be started, so it needs no CAS.
func (d *Driver) Reconcile(ctx context.Context) error {
	all, err := d.goals.List(ctx)
	if err != nil {
		return err
	}
	for _, g := range all {
		exists, err := d.sessions.Exists(ctx, g.SessionID)
		if err != nil {
			return err
		}
		if !exists {
			if err := d.goals.Clear(ctx, g.SessionID); err != nil {
				return err
			}
			continue
		}
		switch g.Status {
		case goal.StatusActive:
			expected := g.Generation
			g.Pause("the runtime restarted — resume to continue", d.now())
			if _, err := d.goals.Save(ctx, g, expected); err != nil {
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

// Quiesce cancels a session's loop so it launches no further runs. It does NOT
// block on the loop's exit — an in-flight run finishes on its own, and the
// generation CAS already makes any straggler write a no-op — so a session
// delete/rollback can quiesce the goal without stalling on a model turn. It is
// the [GoalQuiescer] the session-lifecycle coordinator calls before it clears a
// deleted/rewound session's goal.
func (d *Driver) Quiesce(sessionID string) {
	d.mu.Lock()
	if h := d.running[sessionID]; h != nil {
		h.cancel()
		delete(d.running, sessionID)
	}
	d.mu.Unlock()
}
