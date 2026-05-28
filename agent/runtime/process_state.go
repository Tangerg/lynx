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

func (s *processState) setStatus(st core.AgentProcessStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = st
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

// makeRunning attempts to transition into StatusRunning. Mirrors
// embabel's AbstractAgentProcess.makeRunning(): NotStarted / Waiting /
// Paused all advance into Running, terminal states refuse, and an
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
