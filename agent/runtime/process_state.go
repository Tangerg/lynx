package runtime

import (
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// processState owns the lock-protected mutable state of an AgentProcess
// — the OODA-loop status, the goal currently being pursued, the most
// recent observed world, the action history, the latest failure (if
// any), and the per-process exclusion set used by the planner.
//
// All access goes through the get*/set* methods, which take mu
// internally so callers (AgentProcess and friends) never see the
// lock. processBudget shares mu via a pointer so RecordUsage and
// history appends don't need a second mutex.
type processState struct {
	mu              sync.RWMutex
	status          core.AgentProcessStatus
	goal            *core.Goal
	lastWorld       core.WorldState
	history         []ActionInvocation
	failure         error
	excludedActions map[string]struct{}
}

// newProcessState returns a fresh state block ready for the
// NotStarted → Running transition.
func newProcessState() processState {
	return processState{
		status:          core.StatusNotStarted,
		excludedActions: map[string]struct{}{},
	}
}

func (s *processState) getStatus() core.AgentProcessStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *processState) getGoal() *core.Goal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.goal
}

func (s *processState) getLastWorld() core.WorldState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastWorld
}

func (s *processState) getFailure() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failure
}

// getHistory returns a clone so callers can iterate without racing
// the next recordInvocation.
func (s *processState) getHistory() []ActionInvocation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history)
}

// historyLen reports the action-history length without copying. Used by
// AgentProcess.Usage to avoid materializing the slice when only the
// count is needed.
func (s *processState) historyLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.history)
}

// setStatus transitions to st unless the process is ALREADY terminal —
// terminal is final, so neither a racing kill nor a natural completion can be
// clobbered by a later write (e.g. the run loop reaching completeForGoal after
// KillProcess won, or translateActionStatus parking a process that was just
// killed). Reports whether THIS call performed the transition, so a caller that
// also publishes a terminal event fires it only when it actually won — never a
// duplicate / conflicting terminal. This is the single "first terminal wins"
// gate for every status write except the NotStarted/Waiting/Paused → Running
// entry, which goes through makeRunning's own CAS.
func (s *processState) setStatus(st core.AgentProcessStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.IsTerminal() {
		return false
	}
	s.status = st
	return true
}

func (s *processState) setGoal(g *core.Goal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goal = g
}

func (s *processState) setLastWorld(worldState core.WorldState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastWorld = worldState
}

func (s *processState) setFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = err
}

func (s *processState) recordInvocation(inv ActionInvocation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, inv)
}

func (s *processState) excludeAction(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.excludedActions[name] = struct{}{}
}

func (s *processState) clearExclusions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.excludedActions)
}

func (s *processState) snapshotExclusions() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maps.Clone(s.excludedActions)
}

// makeRunning attempts to transition into StatusRunning.
// NotStarted / Waiting / Paused all advance into Running, terminal states refuse, and an
// already-running process refuses too so concurrent ContinueProcess
// calls don't spawn parallel run loops over the same process. Returns
// true when the caller now owns the run loop.
func (s *processState) makeRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.status {
	case core.StatusRunning,
		core.StatusCompleted, core.StatusFailed, core.StatusStuck,
		core.StatusKilled, core.StatusTerminated:
		return false
	}
	s.status = core.StatusRunning
	return true
}

// markKilled transitions to StatusKilled unless the process is already
// terminal, reporting whether THIS call performed the transition — the external
// kill ([Platform.KillProcess]) side of the shared "first terminal wins" gate.
// It is exactly setStatus(StatusKilled): a kill racing a natural completion (or
// vice versa) can't clobber the other's terminal, and the caller publishes
// ProcessKilled only when it actually won (never a spurious / duplicate one).
func (s *processState) markKilled() bool {
	return s.setStatus(core.StatusKilled)
}
