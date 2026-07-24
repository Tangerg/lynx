// Package admission owns the process-local session admission facts shared by
// Run execution and destructive Session lifecycle operations.
package admission

import "sync"

// Gate serializes one writer per session, records live Runs, and coordinates
// their working-tree admissions with destructive workspace mutations. Its zero
// value is ready to use.
type Gate struct {
	mu            sync.Mutex
	runs          map[string]liveRun
	pending       map[uint64]pendingRun
	claims        map[string]map[uint64]struct{}
	treeRuns      map[string]int
	treeMutations map[string]struct{}
	nextID        uint64
}

type liveRun struct {
	sessionID string
	cwd       string
}

type pendingRun struct {
	sessionID string
	cwd       string
}

// RunAdmission owns a fresh run's session and working-tree reservation until
// it either becomes a live run or is released. Its methods are safe to call
// more than once and across value copies; only the first terminal transition
// takes effect.
type RunAdmission struct {
	lease *runAdmissionLease
}

type runAdmissionLease struct {
	gate *Gate
	id   uint64
	once sync.Once
}

// Admit converts the pending reservation into the live run identified by
// runID. It returns false when the reservation had already been released or
// admitted, or when runID is empty.
func (a RunAdmission) Admit(runID string) bool {
	if a.lease == nil || runID == "" {
		return false
	}
	admitted := false
	a.lease.once.Do(func() {
		g := a.lease.gate
		g.mu.Lock()
		defer g.mu.Unlock()
		pending, ok := g.pending[a.lease.id]
		if !ok {
			return
		}
		delete(g.pending, a.lease.id)
		g.releaseTreeRunLocked(pending.cwd)
		g.runs[runID] = liveRun{sessionID: pending.sessionID, cwd: pending.cwd}
		admitted = true
	})
	return admitted
}

// Release abandons a pending run reservation. It does nothing after Admit.
func (a RunAdmission) Release() {
	if a.lease == nil {
		return
	}
	a.lease.once.Do(func() {
		g := a.lease.gate
		g.mu.Lock()
		defer g.mu.Unlock()
		pending, ok := g.pending[a.lease.id]
		if !ok {
			return
		}
		delete(g.pending, a.lease.id)
		g.releaseTreeRunLocked(pending.cwd)
	})
}

// AcquireSession reserves one session's single-writer slot. Release is safe to
// call more than once and affects only this acquisition.
func (g *Gate) AcquireSession(sessionID string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activeSessionLocked(sessionID) {
		return nil, false
	}
	return g.addClaimLocked(sessionID), true
}

// AcquireRun atomically reserves a fresh run's session and working tree. The
// returned admission must be either admitted after the durable opening commit
// or released when admission fails.
func (g *Gate) AcquireRun(sessionID, cwd string) (RunAdmission, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if g.activeSessionLocked(sessionID) {
		return RunAdmission{}, false
	}
	if cwd != "" {
		if _, busy := g.treeMutations[cwd]; busy {
			return RunAdmission{}, false
		}
		g.addTreeRunLocked(cwd)
	}
	g.nextID++
	id := g.nextID
	g.pending[id] = pendingRun{sessionID: sessionID, cwd: cwd}
	return RunAdmission{lease: &runAdmissionLease{gate: g, id: id}}, true
}

// BeginMaintenance converts a live run into a maintenance reservation. Both
// its session and working tree remain unavailable until Release returns, so a
// checkpoint snapshot cannot race a destructive mutation of the same tree.
func (g *Gate) BeginMaintenance(runID string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	run, ok := g.runs[runID]
	if !ok {
		return nil, false
	}
	delete(g.runs, runID)
	releaseSession := g.addClaimLocked(run.sessionID)
	releaseTree := g.addTreeRunLocked(run.cwd)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseTree()
			releaseSession()
		})
	}, true
}

// AcquireWorkingTreeMutation reserves exclusive access for a destructive
// operation such as a checkpoint restore. It rejects a run while it is pending,
// live, or executing synchronous terminal maintenance on that working tree.
func (g *Gate) AcquireWorkingTreeMutation(cwd string) (release func(), ok bool) {
	if cwd == "" {
		return func() {}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, busy := g.treeMutations[cwd]; busy || g.treeRuns[cwd] > 0 || g.hasLiveRunOnTreeLocked(cwd) {
		return nil, false
	}
	g.treeMutations[cwd] = struct{}{}
	return g.releaseTreeMutation(cwd), true
}

// ActiveSessions snapshots every session with a pending or live Run, or a held
// session-only admission.
func (g *Gate) ActiveSessions() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	set := make(map[string]bool, len(g.runs)+len(g.pending)+len(g.claims))
	for id := range g.claims {
		set[id] = true
	}
	for _, pending := range g.pending {
		set[pending.sessionID] = true
	}
	for _, run := range g.runs {
		set[run.sessionID] = true
	}
	return set
}

func (g *Gate) activeSessionLocked(sessionID string) bool {
	if len(g.claims[sessionID]) > 0 {
		return true
	}
	for _, pending := range g.pending {
		if pending.sessionID == sessionID {
			return true
		}
	}
	for _, run := range g.runs {
		if run.sessionID == sessionID {
			return true
		}
	}
	return false
}

func (g *Gate) initLocked() {
	if g.runs == nil {
		g.runs = map[string]liveRun{}
	}
	if g.pending == nil {
		g.pending = map[uint64]pendingRun{}
	}
	if g.claims == nil {
		g.claims = map[string]map[uint64]struct{}{}
	}
	if g.treeRuns == nil {
		g.treeRuns = map[string]int{}
	}
	if g.treeMutations == nil {
		g.treeMutations = map[string]struct{}{}
	}
}

func (g *Gate) hasLiveRunOnTreeLocked(cwd string) bool {
	for _, run := range g.runs {
		if run.cwd == cwd {
			return true
		}
	}
	return false
}

func (g *Gate) addClaimLocked(sessionID string) func() {
	g.initLocked()
	g.nextID++
	id := g.nextID
	owners := g.claims[sessionID]
	if owners == nil {
		owners = map[uint64]struct{}{}
		g.claims[sessionID] = owners
	}
	owners[id] = struct{}{}

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			owners := g.claims[sessionID]
			delete(owners, id)
			if len(owners) == 0 {
				delete(g.claims, sessionID)
			}
		})
	}
}

func (g *Gate) addTreeRunLocked(cwd string) func() {
	if cwd == "" {
		return func() {}
	}
	g.initLocked()
	g.treeRuns[cwd]++
	return g.releaseTreeRun(cwd)
}

func (g *Gate) releaseTreeRunLocked(cwd string) {
	if cwd == "" {
		return
	}
	if g.treeRuns[cwd] <= 1 {
		delete(g.treeRuns, cwd)
		return
	}
	g.treeRuns[cwd]--
}

func (g *Gate) releaseTreeRun(cwd string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.releaseTreeRunLocked(cwd)
		})
	}
}

func (g *Gate) releaseTreeMutation(cwd string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			delete(g.treeMutations, cwd)
			g.mu.Unlock()
		})
	}
}
