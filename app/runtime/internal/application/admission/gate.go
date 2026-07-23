// Package admission owns the process-local session admission facts shared by
// Run execution and destructive Session lifecycle operations.
package admission

import "sync"

// Gate serializes one writer per session and records the live Run that owns a
// working tree. Its zero value is ready to use.
type Gate struct {
	mu          sync.Mutex
	runs        map[string]liveRun
	claims      map[string]map[uint64]struct{}
	nextClaimID uint64
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

// ActiveSession reports whether a session has a live Run or held admission.
func (g *Gate) ActiveSession(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.activeSessionLocked(sessionID)
}

// ActiveSessionWithCwd returns the session whose live Run owns cwd, if any.
func (g *Gate) ActiveSessionWithCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, run := range g.runs {
		if run.cwd == cwd {
			return run.sessionID
		}
	}
	return ""
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
