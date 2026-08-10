// Package goals owns autonomous Goal execution: given a
// session's objective, it launches runs back-to-back until the model signals the
// goal complete or blocked (through terminal outcome reporting), an opt-in cross-Run
// budget is spent, or the user stops it. It mirrors application/schedules — a
// headless application component that drives the runs Coordinator — but is
// event-driven per goal rather than cron-timed, and consumes each run's terminal
// to decide whether to continue.
//
// Goal driving lives here, NOT in the run pump: the pump holds the session's single
// admission slot across its teardown, so re-entering the coordinator from inside
// it would deadlock. The Driver launches the next run only after the previous
// run's stream has fully drained.
package goals

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/taskgroup"
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
	// ErrClosed reports a lifecycle command after the driver has stopped accepting
	// work. A caller must never be told an active Goal was accepted when no drive
	// can be attached to drive it.
	ErrClosed = errors.New("goals: driver closed")
	// ErrUnavailable reports that this runtime assembled no goal store, so goal
	// mode does not exist here. The driver answers it itself rather than leaving a
	// nil receiver for a caller to remember to check — "not assembled" is a state
	// the owner of the capability reports, not a precondition it delegates.
	ErrUnavailable = errors.New("goals: goal mode unavailable")
)

// Available reports whether goal mode is assembled in this runtime. Nil-safe: a
// runtime with no goal store leaves the driver nil, and asking an absent
// capability whether it exists must answer, not panic.
func (d *Driver) Available() bool { return d != nil && d.goals != nil }

// AutonomousRuns is the Goal driver's narrow view of the Run entry point — the same
// headless start the scheduler uses.
type AutonomousRuns interface {
	WaitSessionStartable(ctx context.Context, sessionID string) error
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
	// Cancel returns after the Run has reached its complete terminal boundary.
	Cancel(ctx context.Context, cmd runs.CancelCommand) (runs.CancelResult, error)
}

// SessionExists reports whether a session id refers to a live session. The
// driver refuses to open a goal for a session that does not exist (no orphan
// goals), and the boot reconcile clears goals whose session was deleted while
// the runtime was down.
type SessionExists interface {
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// RunInstructionInput is the semantic context required to construct one
// autonomous model Run. Goals decides when a first or continuing Run is needed
// without owning model-facing wording.
type RunInstructionInput struct {
	Objective  string
	Continuing bool
}

// RunInstructionBuilder renders the instruction for an autonomous Run.
type RunInstructionBuilder func(RunInstructionInput) string

// Driver owns the per-session autonomous drives. Each active Goal has at most one
// drive goroutine, spawned into a task group so shutdown cancels and joins them.
//
// Every durable goal write is a compare-and-swap on [goal.Version]. An opaque
// lease distinguishes drive ownership across clears, and the revision protects
// mutations inside one lease. Per-session command locks serialize explicit
// lifecycle commands with session write-sets without coupling unrelated
// sessions; drive goroutines and reported outcomes use the store CAS.
type Driver struct {
	goals        Store
	runs         AutonomousRuns
	sessions     SessionExists
	tasks        *taskgroup.Group
	now          func() time.Time
	newLease     func() string
	instructions RunInstructionBuilder

	mutations *SessionMutations
	closed    atomic.Bool
}

// NewDriver builds a Driver sharing one session lifecycle
// coordinator with the sessions use case.
func NewDriver(store Store, autonomousRuns AutonomousRuns, sessions SessionExists, mutations *SessionMutations, instructions RunInstructionBuilder) *Driver {
	if mutations == nil {
		mutations = NewSessionMutations()
	}
	if instructions == nil {
		panic("goals: run instruction builder is required")
	}
	return &Driver{
		goals:        store,
		runs:         autonomousRuns,
		sessions:     sessions,
		tasks:        &taskgroup.Group{},
		now:          time.Now,
		newLease:     uuid.NewString,
		instructions: instructions,
		mutations:    mutations,
	}
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving, and refuses a session that
// does not exist. The new goal gets a fresh lease so a straggler from any
// previously-cleared goal can no longer write.
func (d *Driver) Start(ctx context.Context, sessionID, objective string, selection modelref.Selection, budget goal.Budget) (goal.Goal, error) {
	if !d.Available() {
		return goal.Goal{}, ErrUnavailable
	}
	release := d.mutations.acquire(sessionID)
	defer release()
	if d.closed.Load() {
		return goal.Goal{}, ErrClosed
	}
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
		if err := d.ensureDriveLocked(ctx, sessionID, existing.LeaseID); err != nil {
			return goal.Goal{}, err
		}
		return goal.Goal{}, ErrGoalActive
	}
	if err := d.quiesceDrive(ctx, sessionID); err != nil {
		return goal.Goal{}, err
	}
	// A terminating drive may have committed its final accounting after the
	// first read. Re-read after the ownership boundary so the replacement CAS
	// is based on the complete prior incarnation, never a pre-quiesce snapshot.
	existing, ok, err = d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if ok && existing.Status == goal.StatusActive {
		if err := d.ensureDriveLocked(ctx, sessionID, existing.LeaseID); err != nil {
			return goal.Goal{}, err
		}
		return goal.Goal{}, ErrGoalActive
	}
	var expected goal.Version
	if ok {
		expected = existing.Version()
	}
	g, err := goal.New(sessionID, objective, selection, budget, d.newLease(), d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	g, applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	if !d.launchLocked(ctx, sessionID, g.LeaseID) {
		panic("goals: command crossed the shutdown admission boundary")
	}
	return g, nil
}

// Resume returns a paused or blocked goal to active and drives it again. It is
// idempotent on an already-active goal. The resume renews the lease so
// the fresh drive owns the Goal and any straggler cannot write.
func (d *Driver) Resume(ctx context.Context, sessionID string) (goal.Goal, error) {
	if !d.Available() {
		return goal.Goal{}, ErrUnavailable
	}
	release := d.mutations.acquire(sessionID)
	defer release()
	if d.closed.Load() {
		return goal.Goal{}, ErrClosed
	}
	g, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if g.Status == goal.StatusActive {
		if err := d.ensureDriveLocked(ctx, sessionID, g.LeaseID); err != nil {
			return goal.Goal{}, err
		}
		return g, nil
	}
	if err := d.quiesceDrive(ctx, sessionID); err != nil {
		return goal.Goal{}, err
	}
	g, ok, err = d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if g.Status == goal.StatusActive {
		if err := d.ensureDriveLocked(ctx, sessionID, g.LeaseID); err != nil {
			return goal.Goal{}, err
		}
		return g, nil
	}
	expected := g.Version()
	if err := g.Resume(d.now()); err != nil {
		return goal.Goal{}, err
	}
	g.RenewLease(d.newLease())
	g, applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		return goal.Goal{}, err
	}
	if !applied {
		return goal.Goal{}, ErrGoalConflict
	}
	if !d.launchLocked(ctx, sessionID, g.LeaseID) {
		panic("goals: command crossed the shutdown admission boundary")
	}
	return g, nil
}

// Stop first quiesces the Goal's owned Run, then pauses the authoritative
// post-terminal snapshot. This ordering preserves terminal accounting and makes
// the user stop the final lifecycle transition rather than racing it.
func (d *Driver) Stop(ctx context.Context, sessionID string) (goal.Goal, error) {
	if !d.Available() {
		return goal.Goal{}, ErrUnavailable
	}
	release := d.mutations.acquire(sessionID)
	defer release()
	if d.closed.Load() {
		return goal.Goal{}, ErrClosed
	}
	initial, initiallyPresent, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	wasActive := initiallyPresent && initial.Status == goal.StatusActive
	drive := d.mutations.quiesce(sessionID)
	var quiesceErr error
	if drive != nil {
		quiesceErr = drive.await(ctx)
		if !drive.completed() {
			return goal.Goal{}, quiesceErr
		}
		d.mutations.forget(sessionID, drive)
	}
	current, ok, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, errors.Join(err, quiesceErr)
	}
	if !ok {
		return goal.Goal{}, errors.Join(ErrNoGoal, quiesceErr)
	}
	if !wasActive && current.Status != goal.StatusActive {
		return current, quiesceErr
	}
	expected := current.Version()
	current.Pause(goal.ReasonStoppedByUser, "", d.now())
	current.RenewLease(d.newLease())
	saved, applied, err := d.goals.Save(ctx, current, expected)
	if err != nil {
		return goal.Goal{}, errors.Join(err, quiesceErr)
	}
	if !applied {
		return goal.Goal{}, errors.Join(ErrGoalConflict, quiesceErr)
	}
	return saved, quiesceErr
}

// Current returns the session's Goal, or (zero, false, nil) when it has none.
func (d *Driver) Current(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	if !d.Available() {
		return goal.Goal{}, false, ErrUnavailable
	}
	return d.goals.Get(ctx, sessionID)
}

// quiesceDrive joins a lingering drive before a new Goal incarnation is
// written. The drive stays registered until its goroutine actually exits, so
// a timed-out caller cannot lose the only join point and a later command cannot
// overlap the old Run's admission with a replacement.
func (d *Driver) quiesceDrive(ctx context.Context, sessionID string) error {
	drive := d.mutations.quiesce(sessionID)
	if drive == nil {
		return nil
	}
	err := drive.await(ctx)
	if drive.completed() {
		d.mutations.forget(sessionID, drive)
	}
	return err
}

// Reconcile degrades goals left mid-flight by a previous process. A goal whose
// session no longer exists (deleted while the runtime was down) is cleared — the
// orphan sweep. A live drive cannot survive a restart, so an active Goal becomes
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
			g.Pause(goal.ReasonRuntimeRestarted, "", d.now())
			g.RenewLease(d.newLease())
			if _, _, err := d.goals.Save(ctx, g, expected); err != nil {
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

// BeginShutdown cancels every running Goal drive.
func (d *Driver) BeginShutdown() {
	if d == nil {
		return
	}
	release := d.mutations.acquireAll()
	defer release()
	d.closed.Store(true)
	d.tasks.Cancel()
}

// AwaitShutdown joins every running Goal drive after [BeginShutdown].
func (d *Driver) AwaitShutdown(ctx context.Context) error {
	if d == nil {
		return nil
	}
	return d.tasks.Wait(ctx)
}
