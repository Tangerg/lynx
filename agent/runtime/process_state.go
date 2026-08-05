package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// processState owns a Process's lock-protected mutable state
// — the OODA-loop status, the goal currently being pursued, the most
// recent observed world, the latest failure (if any), and the per-process
// exclusion set used by the planner.
//
// All access goes through methods that own the lock.
type processState struct {
	mu                 sync.RWMutex
	currentStatus      core.ProcessStatus
	currentGoal        *core.Goal
	world              core.WorldState
	runErr             error
	excludedActions    planning.Exclusions
	stuckReplanKey     string
	stuckReplanPending bool
	pendingSuspension  *interaction.Suspension
	runPhase           processRunPhase
	runDone            chan struct{}
	checkpointOwned    bool
}

type processRunPhase uint8

const (
	runIdle processRunPhase = iota
	runDriving
)

// newProcessState returns a fresh state block ready for the
// NotStarted → Running transition.
func newProcessState() processState {
	return processState{
		currentStatus: core.StatusNotStarted,
	}
}

func (s *processState) status() core.ProcessStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentStatus
}

func (s *processState) goal() *core.Goal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentGoal
}

func (s *processState) worldState() core.WorldState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.world
}

func (s *processState) failure() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runErr
}

func (s *processState) suspension() *interaction.Suspension {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pendingSuspension == nil {
		return nil
	}
	return s.pendingSuspension.Clone()
}

// parkSuspension installs exactly one unanswered continuation. A responded
// suspension may be replaced in the same re-entered action, enabling linear
// multi-step HITL without retaining the previous response forever.
func (s *processState) parkSuspension(candidate interaction.Suspension) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return fmt.Errorf("%w: process is terminal", interaction.ErrSuspensionStale)
	}
	if current := s.pendingSuspension; current != nil && !current.Responded() {
		return fmt.Errorf("%w: suspension %q is already pending", interaction.ErrSuspensionConflict, current.ID)
	}
	s.pendingSuspension = candidate.Clone()
	return nil
}

func (s *processState) installClaimedSuspensionResponse(
	id string,
	response json.RawMessage,
) (*interaction.Suspension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.checkpointOwned {
		return nil, ErrProcessCheckpointBusy
	}
	current := s.pendingSuspension
	if s.currentStatus != core.StatusWaiting || current == nil || current.ID != id {
		return nil, fmt.Errorf("%w: process has no pending suspension %q", interaction.ErrSuspensionStale, id)
	}
	if current.Responded() {
		return nil, fmt.Errorf("%w: suspension %q has already been answered", interaction.ErrSuspensionStale, id)
	}
	previous := current.Clone()
	current.Response = bytes.Clone(response)
	return previous, nil
}

func (s *processState) restoreClaimedSuspension(value *interaction.Suspension) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checkpointOwned {
		s.pendingSuspension = value.Clone()
	}
}

func (s *processState) replaceClaimedSuspension(value *interaction.Suspension) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.checkpointOwned || s.currentStatus != core.StatusWaiting {
		return false
	}
	s.pendingSuspension = value.Clone()
	return true
}

func (s *processState) clearRespondedSuspension() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingSuspension != nil && s.pendingSuspension.Responded() {
		s.pendingSuspension = nil
	}
}

// clearContinuableSuspension consumes either an externally answered boundary
// or a framework-ready checkpoint. Callers establish continuability before
// entering this method.
func (s *processState) clearContinuableSuspension() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingSuspension = nil
}

func (s *processState) restoreSuspension(value *interaction.Suspension) error {
	if value == nil {
		return nil
	}
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingSuspension = value.Clone()
	return nil
}

// transition transitions to st unless the process is ALREADY terminal —
// terminal is final, so neither a racing kill nor a natural completion can be
// clobbered by a later write (e.g. the run loop reaching completeForGoal after
// Kill won, or translateActionStatus parking a process that was just
// killed). Reports whether THIS call performed the transition, so a caller that
// also publishes a terminal event fires it only when it actually won — never a
// duplicate / conflicting terminal. This is the single "first terminal wins"
// gate for every status write except the NotStarted/Waiting/Paused → Running
// entry, which goes through beginRun's run-ownership gate.
func (s *processState) transition(status core.ProcessStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return false
	}
	s.currentStatus = status
	if status.IsTerminal() {
		s.pendingSuspension = nil
	}
	return true
}

func (s *processState) pursue(goal *core.Goal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentGoal = goal
}

func (s *processState) observe(worldState core.WorldState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.world = worldState
}

func (s *processState) restoreFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runErr = err
}

func (s *processState) joinFailure(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runErr = errors.Join(s.runErr, err)
}

func (s *processState) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return
	}
	s.runErr = err
	s.currentStatus = core.StatusFailed
	s.pendingSuspension = nil
}

func (s *processState) excludeAction(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.excludedActions = s.excludedActions.With(name)
}

func (s *processState) clearExclusions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.excludedActions = planning.Exclusions{}
}

// beginStuckReplan accepts one recovery attempt for an observed world state.
// Seeing the same state stuck again proves that the policy made no observable
// progress, so the runtime must stop instead of spinning forever.
func (s *processState) beginStuckReplan(worldKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stuckReplanPending && s.stuckReplanKey == worldKey {
		return false
	}
	s.stuckReplanKey = worldKey
	s.stuckReplanPending = true
	return true
}

func (s *processState) clearStuckReplan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stuckReplanKey = ""
	s.stuckReplanPending = false
}

func (s *processState) snapshotExclusions() planning.Exclusions {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.excludedActions
}

// beginRun acquires transient ownership of the run loop and advances a
// resumable stable lifecycle to StatusRunning.
func (s *processState) beginRun() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginRunLocked(false)
}

func (s *processState) beginRunFromCheckpoint() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginRunLocked(true)
}

func (s *processState) beginRunLocked(fromCheckpoint bool) (bool, error) {
	if s.runPhase != runIdle {
		return false, ErrProcessRunning
	}
	if fromCheckpoint && !s.checkpointOwned {
		return false, ErrProcessCheckpointBusy
	}
	if !fromCheckpoint && s.checkpointOwned {
		return false, ErrProcessCheckpointBusy
	}
	switch s.currentStatus {
	case core.StatusCompleted, core.StatusFailed, core.StatusStuck,
		core.StatusKilled, core.StatusTerminated:
		return false, nil
	}
	s.runPhase = runDriving
	s.runDone = make(chan struct{})
	s.currentStatus = core.StatusRunning
	return true, nil
}

type runOutcome struct {
	status  core.ProcessStatus
	failure error
}

// endRun closes the run and reports the indivisible process outcome it
// recorded. Returning one value keeps lifecycle completion distinct from an
// operation that can itself fail; callers that only release ownership need not
// pretend to handle the process failure projection.
func (s *processState) endRun() runOutcome {
	s.mu.Lock()
	done := s.runDone
	outcome := runOutcome{status: s.currentStatus, failure: s.runErr}
	s.runPhase = runIdle
	s.runDone = nil
	s.mu.Unlock()
	if done != nil {
		close(done)
	}
	return outcome
}

func (s *processState) waitRun(ctx context.Context) error {
	s.mu.RLock()
	done := s.runDone
	active := s.runPhase != runIdle
	s.mu.RUnlock()
	if !active || done == nil {
		return nil
	}
	ctx = normalizeContext(ctx)
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return ctx.Err()
		}
	}
}

func (s *processState) runActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runPhase != runIdle
}

func (s *processState) checkpointBusy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.checkpointOwned
}

func (s *processState) claimCheckpoint(allowActiveRun bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checkpointOwned {
		return ErrProcessCheckpointBusy
	}
	if s.runPhase != runIdle && !allowActiveRun {
		return ErrProcessRunning
	}
	s.checkpointOwned = true
	return nil
}

func (s *processState) releaseCheckpoint() {
	s.mu.Lock()
	s.checkpointOwned = false
	s.mu.Unlock()
}

func (s *processState) removable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentStatus.IsTerminal() &&
		s.runPhase == runIdle &&
		!s.checkpointOwned
}

// markKilled transitions to StatusKilled unless the process is already
// terminal, reporting whether THIS call performed the transition — the external
// kill ([Engine.Kill]) side of the shared "first terminal wins" gate.
// A kill racing a natural completion (or vice versa) cannot clobber the
// existing terminal. The winning transition clears any continuation.
func (s *processState) markKilled(err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return false
	}
	s.currentStatus = core.StatusKilled
	s.pendingSuspension = nil
	if err != nil {
		s.runErr = err
	}
	return true
}
