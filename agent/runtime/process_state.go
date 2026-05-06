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
// All access is mediated by mu, which is also shared with the embedded
// [processBudget] so RecordUsage / history appends don't need a second
// mutex (they often happen in the same tick).
//
// processState is embedded in AgentProcess; its non-exported fields
// surface as AgentProcess fields for the rest of runtime to read
// directly (e.g. run.go references p.status / p.goal under p.mu).
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

// --- read-side ------------------------------------------------------------

func (s *processState) Status() core.AgentProcessStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *processState) Goal() *core.Goal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.goal
}

func (s *processState) LastWorldState() core.WorldState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastWorld
}

func (s *processState) Failure() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failure
}

// History returns a snapshot of completed action invocations.
func (s *processState) History() []ActionInvocation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history)
}

// --- write-side -----------------------------------------------------------

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

func (s *processState) setLastWorld(ws core.WorldState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastWorld = ws
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

// makeRunning is the idempotent transition from NotStarted to Running.
// Returns true on the first transition (so the caller knows to start the
// loop) and false thereafter.
func (s *processState) makeRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != core.StatusNotStarted {
		return false
	}
	s.status = core.StatusRunning
	return true
}
