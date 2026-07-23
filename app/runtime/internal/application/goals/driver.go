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
	"time"

	"github.com/google/uuid"

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
	// compare-and-swap; the caller read a version that was already superseded.
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
// Every durable goal write is a compare-and-swap on [goal.Version]. An opaque
// lease distinguishes loop ownership across clears, and the revision protects
// mutations inside one lease. The mutex serializes explicit lifecycle commands
// and session write-sets; loop goroutines and update_goal use the store CAS.
type Driver struct {
	goals    goal.Store
	runs     RunUseCases
	sessions SessionExists
	tasks    *taskgroup.Group
	now      func() time.Time
	newLease func() string

	mutations *SessionMutations
}

type loopHandle struct{ cancel context.CancelFunc }

// NewDriverWithMutations builds a Driver sharing one session lifecycle
// coordinator with the sessions use case.
func NewDriverWithMutations(store goal.Store, runUseCases RunUseCases, sessions SessionExists, mutations *SessionMutations) *Driver {
	if mutations == nil {
		mutations = NewSessionMutations()
	}
	return &Driver{
		goals:     store,
		runs:      runUseCases,
		sessions:  sessions,
		tasks:     &taskgroup.Group{},
		now:       time.Now,
		newLease:  uuid.NewString,
		mutations: mutations,
	}
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving, and refuses a session that
// does not exist. The new goal gets a fresh lease so a straggler from any
// previously-cleared goal can no longer write.
func (d *Driver) Start(ctx context.Context, sessionID, objective, provider, model string, budget goal.Budget) (goal.Goal, error) {
	d.mutations.lock()
	defer d.mutations.unlock()
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
	var expected goal.Version
	if ok {
		expected = existing.Version()
	}
	g, err := goal.New(sessionID, objective, provider, model, budget, d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	g.RenewLease(d.newLease())
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	d.launch(ctx, sessionID, g.LeaseID)
	return g, nil
}

// Resume returns a paused or blocked goal to active and drives it again. It is
// idempotent on an already-active goal. The resume renews the lease so
// the fresh loop owns the goal and any straggler cannot write.
func (d *Driver) Resume(ctx context.Context, sessionID string) (goal.Goal, error) {
	d.mutations.lock()
	defer d.mutations.unlock()
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
	expected := g.Version()
	g.Resume(d.now())
	g.RenewLease(d.newLease())
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	d.launch(ctx, sessionID, g.LeaseID)
	return g, nil
}

// Stop pauses the session's goal and cancels its loop. If the durable pause does
// not commit, it restores a driver for the still-authoritative lease before
// returning the error, so an active Goal never remains without a loop.
func (d *Driver) Stop(ctx context.Context, sessionID string) (goal.Goal, error) {
	d.mutations.lock()
	defer d.mutations.unlock()
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if g.Status != goal.StatusActive {
		return g, nil
	}
	expected := g.Version()
	d.mutations.quiesce(sessionID)
	g.Pause("stopped by the user", d.now())
	g.RenewLease(d.newLease())
	applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		d.recoverDrive(ctx, sessionID, expected.LeaseID)
		return goal.Goal{}, err
	}
	if applied {
		return g, nil
	}
	// The loop or update_goal changed the revision while Stop was quiescing. A
	// restored driver observes the authoritative row and exits unless it remains
	// active under the prior lease.
	d.recoverDrive(ctx, sessionID, expected.LeaseID)
	current, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if ok {
		return current, nil
	}
	return g, nil
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
			expected := g.Version()
			g.Pause("the runtime restarted — resume to continue", d.now())
			g.RenewLease(d.newLease())
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

// WithSessionMutation serializes a session write-set with Start, Resume, and
// Stop. It remains on Driver for callers that own only the Driver; runtime
// assembly shares the same coordinator directly with sessions.
func (d *Driver) WithSessionMutation(ctx context.Context, sessionIDs []string, apply func(context.Context) error) error {
	return d.mutations.WithSessionMutation(ctx, sessionIDs, apply)
}

func (d *Driver) recoverDrive(ctx context.Context, sessionID, leaseID string) {
	d.launch(ctx, sessionID, leaseID)
}
