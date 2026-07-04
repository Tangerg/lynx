package server

import (
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
)

type workingTreeAdmission struct {
	release func()
}

func (a workingTreeAdmission) Release() {
	if a.release != nil {
		a.release()
	}
}

type workingTreeGate struct {
	mu        sync.Mutex
	runCount  map[string]int
	mutations map[string]struct{}
}

func (g *workingTreeGate) ClaimRun(cwd string) (workingTreeAdmission, bool) {
	cwd = fspath.Canonical(cwd)
	if cwd == "" {
		return workingTreeAdmission{}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, ok := g.mutations[cwd]; ok {
		return workingTreeAdmission{}, false
	}
	g.runCount[cwd]++
	return workingTreeAdmission{release: func() { g.releaseRun(cwd) }}, true
}

func (g *workingTreeGate) ClaimMutation(cwd string) (workingTreeAdmission, bool) {
	cwd = fspath.Canonical(cwd)
	if cwd == "" {
		return workingTreeAdmission{}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	if _, ok := g.mutations[cwd]; ok {
		return workingTreeAdmission{}, false
	}
	if g.runCount[cwd] > 0 {
		return workingTreeAdmission{}, false
	}
	g.mutations[cwd] = struct{}{}
	return workingTreeAdmission{release: func() { g.releaseMutation(cwd) }}, true
}

func (g *workingTreeGate) releaseRun(cwd string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.runCount[cwd] <= 1 {
		delete(g.runCount, cwd)
		return
	}
	g.runCount[cwd]--
}

func (g *workingTreeGate) releaseMutation(cwd string) {
	g.mu.Lock()
	delete(g.mutations, cwd)
	g.mu.Unlock()
}

func (g *workingTreeGate) initLocked() {
	if g.runCount == nil {
		g.runCount = map[string]int{}
	}
	if g.mutations == nil {
		g.mutations = map[string]struct{}{}
	}
}

func (s *Server) claimWorkingTreeRun(cwd string) (workingTreeAdmission, bool) {
	return s.workingTrees.ClaimRun(cwd)
}

func (s *Server) claimWorkingTreeMutation(cwd string) (workingTreeAdmission, bool) {
	return s.workingTrees.ClaimMutation(cwd)
}
