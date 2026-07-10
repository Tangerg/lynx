package lifecycle

import "sync"

// WorkingTreeAdmission is a held working-tree slot. Release drops it once and
// is idempotent across value copies.
type WorkingTreeAdmission struct {
	release *releaseOnce
}

// Release drops the held working-tree slot.
func (a WorkingTreeAdmission) Release() {
	if a.release != nil {
		a.release.run()
	}
}

// WorkingTreeGate coordinates short run admissions with destructive
// working-tree mutations for one runtime process. Callers pass a canonical cwd;
// empty cwd means no working tree to coordinate.
type WorkingTreeGate struct {
	mu        sync.Mutex
	runCount  map[string]int
	mutations map[string]struct{}
}

// ClaimRun reserves a working tree for a run segment admission.
func (g *WorkingTreeGate) ClaimRun(cwd string) (WorkingTreeAdmission, bool) {
	if cwd == "" {
		return WorkingTreeAdmission{}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, ok := g.mutations[cwd]; ok {
		return WorkingTreeAdmission{}, false
	}
	g.runCount[cwd]++
	return WorkingTreeAdmission{release: newReleaseOnce(func() { g.releaseRun(cwd) })}, true
}

// ClaimMutation reserves exclusive access for a destructive working-tree
// mutation such as file rollback.
func (g *WorkingTreeGate) ClaimMutation(cwd string) (WorkingTreeAdmission, bool) {
	if cwd == "" {
		return WorkingTreeAdmission{}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, ok := g.mutations[cwd]; ok {
		return WorkingTreeAdmission{}, false
	}
	if g.runCount[cwd] > 0 {
		return WorkingTreeAdmission{}, false
	}
	g.mutations[cwd] = struct{}{}
	return WorkingTreeAdmission{release: newReleaseOnce(func() { g.releaseMutation(cwd) })}, true
}

func (g *WorkingTreeGate) releaseRun(cwd string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.runCount[cwd] <= 1 {
		delete(g.runCount, cwd)
		return
	}
	g.runCount[cwd]--
}

func (g *WorkingTreeGate) releaseMutation(cwd string) {
	g.mu.Lock()
	delete(g.mutations, cwd)
	g.mu.Unlock()
}

func (g *WorkingTreeGate) initLocked() {
	if g.runCount == nil {
		g.runCount = map[string]int{}
	}
	if g.mutations == nil {
		g.mutations = map[string]struct{}{}
	}
}
