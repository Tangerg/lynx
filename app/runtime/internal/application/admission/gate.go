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
	claims        map[string]map[uint64]struct{}
	treeRuns      map[string]int
	treeMutations map[string]struct{}
	nextClaimID   uint64
}

type liveRun struct {
	sessionID string
	cwd       string
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

// OpenRun records the session and working tree held by a live Run segment.
func (g *Gate) OpenRun(runID, sessionID, cwd string) {
	g.mu.Lock()
	g.initLocked()
	g.runs[runID] = liveRun{sessionID: sessionID, cwd: cwd}
	g.mu.Unlock()
}

// BeginMaintenance removes a completed segment and retains its session slot
// through the synchronous boundary maintenance that follows. Release drops that
// maintenance claim.
func (g *Gate) BeginMaintenance(runID string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	run, ok := g.runs[runID]
	if !ok {
		return nil, false
	}
	delete(g.runs, runID)
	return g.addClaimLocked(run.sessionID), true
}

// AcquireWorkingTreeRun reserves cwd while a run segment is being admitted.
// A live run keeps the tree unavailable to destructive mutations after this
// short admission claim is released.
func (g *Gate) AcquireWorkingTreeRun(cwd string) (release func(), ok bool) {
	if cwd == "" {
		return func() {}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, busy := g.treeMutations[cwd]; busy {
		return nil, false
	}
	g.treeRuns[cwd]++
	return g.releaseTreeRun(cwd), true
}

// AcquireWorkingTreeMutation reserves exclusive access for a destructive
// operation such as a checkpoint restore. It rejects both a segment still being
// admitted and every live run sharing the same working tree.
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

// ActiveSessions snapshots every session with a live Run or held admission.
func (g *Gate) ActiveSessions() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	set := make(map[string]bool, len(g.runs)+len(g.claims))
	for id := range g.claims {
		set[id] = true
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
	g.nextClaimID++
	id := g.nextClaimID
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

func (g *Gate) releaseTreeRun(cwd string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.treeRuns[cwd] <= 1 {
				delete(g.treeRuns, cwd)
				return
			}
			g.treeRuns[cwd]--
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
