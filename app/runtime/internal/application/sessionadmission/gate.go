// Package sessionadmission owns the session admission facts shared by Run
// execution and destructive Session lifecycle operations. A configured
// Ownership extends the same invariants across Runtime processes.
package sessionadmission

import (
	"context"
	"errors"
	"sync"
)

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
	changed       chan struct{}
	nextID        uint64
	ownership     Ownership
}

// Lease is a held cross-process ownership claim.
type Lease interface {
	Release()
}

// Ownership maps product identities to non-blocking cross-process leases.
// Implementations live outside Application and must fail closed.
type Ownership interface {
	TrySession(sessionID string) (Lease, bool)
	TryWorkingTree(cwd string, shared bool) (Lease, bool)
}

// New constructs a Gate whose single-writer and working-tree invariants span
// every Runtime process sharing the ownership backend. A nil owner keeps the
// zero-value process-local behavior used by isolated tests.
func New(ownership Ownership) *Gate { return &Gate{ownership: ownership} }

type liveRun struct {
	sessionID string
	cwd       string
	leases    []Lease
}

type pendingRun struct {
	sessionID string
	cwd       string
	leases    []Lease
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
func (r RunAdmission) Admit(runID string) bool {
	if r.lease == nil || runID == "" {
		return false
	}
	admitted := false
	r.lease.once.Do(func() {
		g := r.lease.gate
		g.mu.Lock()
		defer g.mu.Unlock()
		pending, ok := g.pending[r.lease.id]
		if !ok {
			return
		}
		delete(g.pending, r.lease.id)
		g.releaseTreeRunLocked(pending.cwd)
		g.runs[runID] = liveRun(pending)
		admitted = true
	})
	return admitted
}

// Release abandons a pending run reservation. It does nothing after Admit.
func (r RunAdmission) Release() {
	if r.lease == nil {
		return
	}
	r.lease.once.Do(func() {
		g := r.lease.gate
		g.mu.Lock()
		pending, ok := g.pending[r.lease.id]
		if !ok {
			g.mu.Unlock()
			return
		}
		delete(g.pending, r.lease.id)
		g.releaseTreeRunLocked(pending.cwd)
		g.notifyLocked()
		g.mu.Unlock()
		releaseLeases(pending.leases)
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
	lease, ok := g.trySessionLease(sessionID)
	if !ok {
		return nil, false
	}
	releaseLocal := g.addClaimLocked(sessionID)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseLocal()
			lease.Release()
		})
	}, true
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
	sessionLease, ok := g.trySessionLease(sessionID)
	if !ok {
		return RunAdmission{}, false
	}
	leases := []Lease{sessionLease}
	if cwd != "" {
		if _, busy := g.treeMutations[cwd]; busy {
			releaseLeases(leases)
			return RunAdmission{}, false
		}
		treeLease, acquired := g.tryWorkingTreeLease(cwd, true)
		if !acquired {
			releaseLeases(leases)
			return RunAdmission{}, false
		}
		leases = append(leases, treeLease)
		g.addTreeRunLocked(cwd)
	}
	g.nextID++
	id := g.nextID
	g.pending[id] = pendingRun{sessionID: sessionID, cwd: cwd, leases: leases}
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
			releaseLeases(run.leases)
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
	lease, ok := g.tryWorkingTreeLease(cwd, false)
	if !ok {
		return nil, false
	}
	g.treeMutations[cwd] = struct{}{}
	releaseLocal := g.releaseTreeMutation(cwd)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseLocal()
			lease.Release()
		})
	}, true
}

func (g *Gate) trySessionLease(sessionID string) (Lease, bool) {
	if g.ownership == nil {
		return noopLease{}, true
	}
	return g.ownership.TrySession(sessionID)
}

func (g *Gate) tryWorkingTreeLease(cwd string, shared bool) (Lease, bool) {
	if g.ownership == nil {
		return noopLease{}, true
	}
	return g.ownership.TryWorkingTree(cwd, shared)
}

type noopLease struct{}

func (noopLease) Release() {}

func releaseLeases(leases []Lease) {
	for index := len(leases) - 1; index >= 0; index-- {
		leases[index].Release()
	}
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

// WaitRunStartable blocks until sessionID has no pending, live, maintenance, or
// session-only admission and cwd has no destructive working-tree mutation. It
// is an observation boundary, not a reservation: callers must still acquire
// their own Run admission after it returns and may wait again if another owner
// wins that race.
func (g *Gate) WaitRunStartable(ctx context.Context, sessionID, cwd string) error {
	if ctx == nil {
		return errors.New("session admission: wait context is required")
	}
	for {
		g.mu.Lock()
		g.initLocked()
		_, treeMutation := g.treeMutations[cwd]
		if !g.activeSessionLocked(sessionID) && (cwd == "" || !treeMutation) {
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		g.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
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
	if g.changed == nil {
		g.changed = make(chan struct{})
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
			g.notifyLocked()
		})
	}
}

func (g *Gate) notifyLocked() {
	if g.changed == nil {
		g.changed = make(chan struct{})
		return
	}
	close(g.changed)
	g.changed = make(chan struct{})
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
			defer g.mu.Unlock()
			delete(g.treeMutations, cwd)
			g.notifyLocked()
		})
	}
}
