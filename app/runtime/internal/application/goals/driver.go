// Package goals owns autonomous Goal execution: given a
// session's objective, it launches runs back-to-back until the model signals the
// goal complete or blocked (through terminal outcome reporting), an opt-in cross-Run
// budget is spent, or the user stops it. It mirrors application/schedules — a
// autonomous application component that drives the runs Coordinator — but is
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
	"github.com/Tangerg/lynx/app/runtime/internal/application/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
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
	// ErrGoalOwned reports that another Runtime process owns the autonomous
	// drive for this Session. The durable Goal remains readable, but lifecycle
	// mutation must not create a second driver.
	ErrGoalOwned = errors.New("goals: goal drive is owned by another runtime")
	// ErrClosed reports a lifecycle command after the driver has stopped accepting
	// work. A caller must never be told an active Goal was accepted when no drive
	// can be attached to drive it.
	ErrClosed = errors.New("goals: driver closed")
	// ErrUnavailable reports that this runtime assembled no goal store, so goal
	// mode does not exist here. The driver answers it itself rather than leaving a
	// nil receiver for a caller to remember to check — "not assembled" is a state
	// the owner of the capability reports, not a precondition it delegates.
	ErrUnavailable = errors.New("goals: goal mode unavailable")
	// ErrInsufficientCapabilities reports a caller that cannot observe every
	// optional behavior already frozen on the Goal incarnation.
	ErrInsufficientCapabilities = errors.New("goals: caller capabilities are insufficient")
)

// InsufficientCapabilitiesError names the complete frozen capability gap.
type InsufficientCapabilitiesError struct {
	SessionID string
	Missing   run.Capabilities
}

func (i *InsufficientCapabilitiesError) Error() string {
	return i.SessionID + ": " + ErrInsufficientCapabilities.Error() + ": " + i.Missing.String()
}

func (i *InsufficientCapabilitiesError) Is(target error) bool {
	return target == ErrInsufficientCapabilities
}

// Available reports whether goal mode is assembled in this runtime. Nil-safe: a
// runtime with no goal store leaves the driver nil, and asking an absent
// capability whether it exists must answer, not panic.
func (d *Driver) Available() bool { return d != nil && d.goals != nil }

// AutonomousRuns is the Goal driver's narrow view of the Run entry point.
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

// DriveLease is the cross-process ownership of one Session's autonomous Goal
// driver. Release is idempotent; process death also releases the underlying OS
// lease.
type DriveLease interface {
	Release()
}

// DriveOwnership supplies non-blocking leases without leaking filesystem or
// process mechanisms into Application.
type DriveOwnership interface {
	TryGoalDrive(sessionID string) (DriveLease, bool)
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
// incarnation distinguishes fresh objectives across clears, and the revision
// protects mutations inside one incarnation. Per-session command locks serialize explicit
// lifecycle commands with session write-sets without coupling unrelated
// sessions; drive goroutines and reported outcomes use the store CAS.
type Driver struct {
	goals          Store
	runs           AutonomousRuns
	sessions       SessionExists
	tasks          *taskgroup.Group
	now            func() time.Time
	newIncarnation func() string
	instructions   RunInstructionBuilder

	mutations *SessionMutations
	ownership DriveOwnership
	closed    atomic.Bool
}

// NewDriver builds a Driver sharing one session lifecycle
// coordinator with the sessions use case.
func NewDriver(
	store Store,
	autonomousRuns AutonomousRuns,
	sessions SessionExists,
	mutations *SessionMutations,
	ownership DriveOwnership,
	instructions RunInstructionBuilder,
) *Driver {
	if mutations == nil {
		mutations = NewSessionMutations()
	}
	if instructions == nil {
		panic("goals: run instruction builder is required")
	}
	return &Driver{
		goals:          store,
		runs:           autonomousRuns,
		sessions:       sessions,
		tasks:          &taskgroup.Group{},
		now:            time.Now,
		newIncarnation: uuid.NewString,
		instructions:   instructions,
		mutations:      mutations,
		ownership:      ownership,
	}
}

// Start opens a new goal for the session and begins driving it. It replaces a
// paused or blocked goal (a fresh objective abandons the old one) but refuses to
// clobber a goal that is already actively driving, and refuses a session that
// does not exist. The new objective gets a fresh incarnation so a Run from any
// previously-cleared Goal can no longer write it.
func (d *Driver) Start(
	ctx context.Context,
	sessionID, objective string,
	selection modelref.Selection,
	budget goal.Budget,
	capabilities run.Capabilities,
) (goal.Goal, error) {
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
		if ensureDriveLockedErr := d.ensureDriveLocked(ctx, sessionID, existing.IncarnationID); ensureDriveLockedErr != nil {
			return goal.Goal{}, ensureDriveLockedErr
		}
		return goal.Goal{}, ErrGoalActive
	}
	if quiesceDriveErr := d.quiesceDrive(ctx, sessionID); quiesceDriveErr != nil {
		return goal.Goal{}, quiesceDriveErr
	}
	// A terminating drive may have committed its final accounting after the
	// first read. Re-read after the ownership boundary so the replacement CAS
	// is based on the complete prior incarnation, never a pre-quiesce snapshot.
	existing, ok, err = d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if ok && existing.Status == goal.StatusActive {
		if ensureDriveLockedErr := d.ensureDriveLocked(ctx, sessionID, existing.IncarnationID); ensureDriveLockedErr != nil {
			return goal.Goal{}, ensureDriveLockedErr
		}
		return goal.Goal{}, ErrGoalActive
	}
	var expected goal.Version
	if ok {
		expected = existing.Version()
	}
	g, err := goal.New(sessionID, objective, selection, budget, capabilities, d.newIncarnation(), d.now())
	if err != nil {
		return goal.Goal{}, err
	}
	driveLease, ok := d.tryDriveLease(sessionID)
	if !ok {
		return goal.Goal{}, ErrGoalOwned
	}
	g, applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		driveLease.Release()
		return goal.Goal{}, err
	}
	if !applied {
		driveLease.Release()
		return goal.Goal{}, ErrGoalConflict
	}
	if err := d.launchLocked(ctx, sessionID, g.IncarnationID, driveLease); err != nil {
		panic("goals: command crossed the shutdown admission boundary")
	}
	return g, nil
}

// Resume returns a paused or blocked goal to active and drives it again. It is
// idempotent on an already-active goal. Resume preserves the objective
// incarnation: a Run parked for HITL remains part of this Goal when it resumes,
// while quiesceDrive is the process-local boundary between old and new drives.
func (d *Driver) Resume(ctx context.Context, sessionID string, caller run.Capabilities) (goal.Goal, error) {
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
	if missing := g.Capabilities.MissingFrom(caller); !missing.IsEmpty() {
		return goal.Goal{}, &InsufficientCapabilitiesError{SessionID: sessionID, Missing: missing}
	}
	if g.Status == goal.StatusActive {
		if ensureDriveLockedErr := d.ensureDriveLocked(ctx, sessionID, g.IncarnationID); ensureDriveLockedErr != nil {
			return goal.Goal{}, ensureDriveLockedErr
		}
		return g, nil
	}
	if quiesceDriveErr := d.quiesceDrive(ctx, sessionID); quiesceDriveErr != nil {
		return goal.Goal{}, quiesceDriveErr
	}
	g, ok, err = d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !ok {
		return goal.Goal{}, ErrNoGoal
	}
	if missing := g.Capabilities.MissingFrom(caller); !missing.IsEmpty() {
		return goal.Goal{}, &InsufficientCapabilitiesError{SessionID: sessionID, Missing: missing}
	}
	if g.Status == goal.StatusActive {
		if ensureDriveLockedErr := d.ensureDriveLocked(ctx, sessionID, g.IncarnationID); ensureDriveLockedErr != nil {
			return goal.Goal{}, ensureDriveLockedErr
		}
		return g, nil
	}
	expected := g.Version()
	if resumeErr := g.Resume(d.now()); resumeErr != nil {
		return goal.Goal{}, resumeErr
	}
	driveLease, ok := d.tryDriveLease(sessionID)
	if !ok {
		return goal.Goal{}, ErrGoalOwned
	}
	g, applied, err := d.goals.Save(ctx, g, expected)
	if err != nil {
		driveLease.Release()
		return goal.Goal{}, err
	}
	if !applied {
		driveLease.Release()
		return goal.Goal{}, ErrGoalConflict
	}
	if err := d.launchLocked(ctx, sessionID, g.IncarnationID, driveLease); err != nil {
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
	var foreignLease DriveLease
	if wasActive && drive == nil {
		var ok bool
		foreignLease, ok = d.tryDriveLease(sessionID)
		if !ok {
			return goal.Goal{}, ErrGoalOwned
		}
		defer foreignLease.Release()
	}
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
	saved, applied, err := d.goals.Save(ctx, current, expected)
	if err != nil {
		return goal.Goal{}, errors.Join(err, quiesceErr)
	}
	if !applied {
		return goal.Goal{}, errors.Join(ErrGoalConflict, quiesceErr)
	}
	return saved, quiesceErr
}

// UpdateObjective quiesces any locally owned drive before replacing the text.
// The replacement keeps lifecycle and accounting but receives a fresh
// incarnation, so a Run admitted for the previous objective cannot charge or
// transition it. An objective that was active before quiescence continues when
// its canceled Run did not independently block or complete the Goal.
func (d *Driver) UpdateObjective(
	ctx context.Context,
	sessionID, objective string,
	caller run.Capabilities,
) (goal.Goal, error) {
	if !d.Available() {
		return goal.Goal{}, ErrUnavailable
	}
	release := d.mutations.acquire(sessionID)
	defer release()
	if d.closed.Load() {
		return goal.Goal{}, ErrClosed
	}
	initial, present, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	if !present {
		return goal.Goal{}, ErrNoGoal
	}
	if initial.Status == goal.StatusComplete {
		return goal.Goal{}, goal.ErrNotEditable
	}
	wasActive := initial.Status == goal.StatusActive
	if wasActive {
		if missing := initial.Capabilities.MissingFrom(caller); !missing.IsEmpty() {
			return goal.Goal{}, &InsufficientCapabilitiesError{SessionID: sessionID, Missing: missing}
		}
	}

	drive := d.mutations.quiesce(sessionID)
	var commandLease DriveLease
	if wasActive && drive == nil {
		var acquired bool
		commandLease, acquired = d.tryDriveLease(sessionID)
		if !acquired {
			return goal.Goal{}, ErrGoalOwned
		}
		defer func() {
			if commandLease != nil {
				commandLease.Release()
			}
		}()
	}
	var quiesceErr error
	if drive != nil {
		quiesceErr = drive.await(ctx)
		if !drive.completed() {
			return goal.Goal{}, quiesceErr
		}
		d.mutations.forget(sessionID, drive)
	}

	current, present, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, errors.Join(err, quiesceErr)
	}
	if !present {
		return goal.Goal{}, errors.Join(ErrNoGoal, quiesceErr)
	}
	if current.Status == goal.StatusComplete {
		return goal.Goal{}, errors.Join(goal.ErrNotEditable, quiesceErr)
	}
	expected := current.Version()
	if reviseObjectiveErr := current.ReviseObjective(objective, d.newIncarnation(), d.now()); reviseObjectiveErr != nil {
		return goal.Goal{}, errors.Join(reviseObjectiveErr, quiesceErr)
	}
	if wasActive && current.Status == goal.StatusPaused {
		if resumeErr := current.Resume(d.now()); resumeErr != nil {
			return goal.Goal{}, errors.Join(resumeErr, quiesceErr)
		}
	}

	if current.Status == goal.StatusActive && commandLease == nil {
		var acquired bool
		commandLease, acquired = d.tryDriveLease(sessionID)
		if !acquired {
			return goal.Goal{}, errors.Join(ErrGoalOwned, quiesceErr)
		}
	}
	saved, applied, err := d.goals.Save(ctx, current, expected)
	if err != nil {
		if commandLease != nil {
			commandLease.Release()
			commandLease = nil
		}
		return goal.Goal{}, errors.Join(err, quiesceErr)
	}
	if !applied {
		if commandLease != nil {
			commandLease.Release()
			commandLease = nil
		}
		return goal.Goal{}, errors.Join(ErrGoalConflict, quiesceErr)
	}
	if saved.Status == goal.StatusActive {
		if err := d.launchLocked(ctx, sessionID, saved.IncarnationID, commandLease); err != nil {
			panic("goals: command crossed the shutdown admission boundary")
		}
		commandLease = nil
	} else if commandLease != nil {
		commandLease.Release()
		commandLease = nil
	}
	return saved, quiesceErr
}

// Clear quiesces the owned drive and conditionally removes the authoritative
// Goal aggregate. An already-absent Goal is a successful idempotent clear, which
// lets a stale UI intent converge with automatic completion.
func (d *Driver) Clear(ctx context.Context, sessionID string) error {
	if !d.Available() {
		return ErrUnavailable
	}
	release := d.mutations.acquire(sessionID)
	defer release()
	if d.closed.Load() {
		return ErrClosed
	}
	initial, present, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	wasActive := initial.Status == goal.StatusActive
	drive := d.mutations.quiesce(sessionID)
	var commandLease DriveLease
	if wasActive && drive == nil {
		var acquired bool
		commandLease, acquired = d.tryDriveLease(sessionID)
		if !acquired {
			return ErrGoalOwned
		}
		defer commandLease.Release()
	}
	var quiesceErr error
	if drive != nil {
		quiesceErr = drive.await(ctx)
		if !drive.completed() {
			return quiesceErr
		}
		d.mutations.forget(sessionID, drive)
	}
	current, present, err := d.goals.Get(ctx, sessionID)
	if err != nil {
		return errors.Join(err, quiesceErr)
	}
	if !present {
		return quiesceErr
	}
	if current.Status == goal.StatusActive && commandLease == nil {
		var acquired bool
		commandLease, acquired = d.tryDriveLease(sessionID)
		if !acquired {
			return errors.Join(ErrGoalOwned, quiesceErr)
		}
		defer commandLease.Release()
	}
	applied, err := d.goals.ClearIf(ctx, sessionID, current.Version())
	if err != nil {
		return errors.Join(err, quiesceErr)
	}
	if !applied {
		return errors.Join(ErrGoalConflict, quiesceErr)
	}
	return quiesceErr
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

// Reconcile degrades Goals whose drive owner died. A goal whose Session no
// longer exists is cleared; an abandoned active Goal becomes paused rather than
// silently resuming and burning budget; a Goal caught at transient complete is
// cleared. The same cross-process lease held by a live drive makes startup and
// survivor sweeps skip it. Every transition still uses the listed version and
// fails closed on a CAS miss.
func (d *Driver) Reconcile(ctx context.Context) error {
	all, err := d.goals.List(ctx)
	if err != nil {
		return err
	}
	for _, g := range all {
		lease, acquired := d.tryDriveLease(g.SessionID)
		if !acquired {
			// A different Runtime still owns the live drive. Its Goal and Run
			// facts are not crash leftovers for this process to rewrite.
			continue
		}
		exists, err := d.sessions.Exists(ctx, g.SessionID)
		if err != nil {
			lease.Release()
			return err
		}
		if !exists {
			applied, err := d.goals.ClearIf(ctx, g.SessionID, g.Version())
			if err != nil {
				lease.Release()
				return err
			}
			if !applied {
				lease.Release()
				return ErrGoalConflict
			}
			lease.Release()
			continue
		}
		switch g.Status {
		case goal.StatusActive:
			expected := g.Version()
			g.Pause(goal.ReasonRuntimeRestarted, "", d.now())
			if _, applied, err := d.goals.Save(ctx, g, expected); err != nil {
				lease.Release()
				return err
			} else if !applied {
				lease.Release()
				return ErrGoalConflict
			}
		case goal.StatusComplete:
			applied, err := d.goals.ClearIf(ctx, g.SessionID, g.Version())
			if err != nil {
				lease.Release()
				return err
			}
			if !applied {
				lease.Release()
				return ErrGoalConflict
			}
		}
		lease.Release()
	}
	return nil
}

func (d *Driver) tryDriveLease(sessionID string) (DriveLease, bool) {
	if d.ownership == nil {
		return noopDriveLease{}, true
	}
	return d.ownership.TryGoalDrive(sessionID)
}

type noopDriveLease struct{}

func (noopDriveLease) Release() {}

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
