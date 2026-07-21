package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// processState owns a Process's lock-protected mutable state
// — the OODA-loop status, the goal currently being pursued, the most
// recent observed world, the action history, the latest failure (if
// any), and the per-process exclusion set used by the planner.
//
// All access goes through methods that own the lock. processBudget shares mu
// so accounting and action history remain one consistent aggregate.
type processState struct {
	mu                 sync.RWMutex
	currentStatus      core.ProcessStatus
	currentGoal        *core.Goal
	world              core.WorldState
	history            []ActionRun
	runErr             error
	excludedActions    planning.Exclusions
	stuckReplanKey     string
	stuckReplanPending bool
	pendingSuspension  *interaction.Suspension
	revision           uint64
	runOwned           bool
}

func (s *processState) snapshotRevision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

func (s *processState) restoreRevision(revision uint64) {
	s.mu.Lock()
	s.revision = revision
	s.mu.Unlock()
}

func (s *processState) commitRevision(revision uint64) {
	s.mu.Lock()
	s.revision = revision
	s.mu.Unlock()
}

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
		if current.ID == candidate.ID && suspensionEqual(*current, candidate) {
			return nil
		}
		return fmt.Errorf("%w: suspension %q is already pending", interaction.ErrSuspensionConflict, current.ID)
	}
	s.pendingSuspension = candidate.Clone()
	return nil
}

func (s *processState) respondToSuspension(id string, response any, now time.Time) error {
	s.mu.RLock()
	current := s.pendingSuspension
	status := s.currentStatus
	if current != nil {
		current = current.Clone()
	}
	s.mu.RUnlock()
	if status != core.StatusWaiting || current == nil || current.ID != id {
		return fmt.Errorf("%w: process has no pending suspension %q", interaction.ErrSuspensionStale, id)
	}
	canonical, err := current.ValidateResponse(response)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current = s.pendingSuspension
	if s.currentStatus != core.StatusWaiting || current == nil || current.ID != id {
		return fmt.Errorf("%w: suspension %q changed before response", interaction.ErrSuspensionStale, id)
	}
	if current.Responded() {
		if current.SameResponse(canonical) {
			return nil
		}
		return fmt.Errorf("%w: suspension %q already has a different response", interaction.ErrSuspensionConflict, id)
	}
	current.Response = bytes.Clone(canonical)
	current.RespondedAt = now
	return nil
}

func (s *processState) clearRespondedSuspension() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingSuspension != nil && s.pendingSuspension.Responded() {
		s.pendingSuspension = nil
	}
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

func suspensionEqual(a, b interaction.Suspension) bool {
	a.CreatedAt = b.CreatedAt
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

// historySnapshot returns a clone so callers can iterate without racing
// the next recordActionRun.
func (s *processState) historySnapshot() []ActionRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history)
}

// historyLen reports the action-history length without copying. Used by
// Process.Usage to avoid materializing the slice when only the
// count is needed.
func (s *processState) historyLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.history)
}

// transition transitions to st unless the process is ALREADY terminal —
// terminal is final, so neither a racing kill nor a natural completion can be
// clobbered by a later write (e.g. the run loop reaching completeForGoal after
// Kill won, or translateActionStatus parking a process that was just
// killed). Reports whether THIS call performed the transition, so a caller that
// also publishes a terminal event fires it only when it actually won — never a
// duplicate / conflicting terminal. This is the single "first terminal wins"
// gate for every status write except the NotStarted/Waiting/Paused → Running
// entry, which goes through beginRun's own CAS.
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

func (s *processState) fail(err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return false
	}
	s.runErr = err
	s.currentStatus = core.StatusFailed
	s.pendingSuspension = nil
	return true
}

func (s *processState) pauseDurability() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return false
	}
	if s.currentStatus == core.StatusWaiting || s.currentStatus == core.StatusPaused {
		return true
	}
	s.currentStatus = core.StatusPaused
	return true
}

func (s *processState) recordActionRun(run ActionRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, run)
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
// resumable lifecycle to StatusRunning. A durable Running snapshot has no live
// owner after restore and is therefore resumable; a live owner is rejected.
func (s *processState) beginRun() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runOwned {
		return false, ErrProcessRunning
	}
	switch s.currentStatus {
	case core.StatusCompleted, core.StatusFailed, core.StatusStuck,
		core.StatusKilled, core.StatusTerminated:
		return false, nil
	}
	s.runOwned = true
	s.currentStatus = core.StatusRunning
	return true, nil
}

func (s *processState) endRun() {
	s.mu.Lock()
	s.runOwned = false
	s.mu.Unlock()
}

// markKilled transitions to StatusKilled unless the process is already
// terminal, reporting whether THIS call performed the transition — the external
// kill ([Engine.Kill]) side of the shared "first terminal wins" gate.
// A kill racing a natural completion (or vice versa) cannot clobber the
// existing terminal. The winning transition clears any continuation and
// reports whether the run loop owns final snapshot responsibility.
func (s *processState) markKilled(err error) (won, runOwned bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentStatus.IsTerminal() {
		return false, s.runOwned
	}
	s.currentStatus = core.StatusKilled
	s.pendingSuspension = nil
	if err != nil {
		s.runErr = err
	}
	return true, s.runOwned
}
