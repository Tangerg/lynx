package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type coordinatorStores struct {
	interrupts    *coordinatorInterrupts
	snapshot      Snapshot
	terminal      *TerminalPlan
	forked        *ForkPlan
	rolledBack    *RollbackPlan
	snapshotReads *int
}

type testStores interface {
	WriteSets
	Session() Store
	Interrupts() InterruptStore
	Transcript() TranscriptStore
	Runs() RunStore
	ReadSnapshot(context.Context, string) (Snapshot, error)
	ForgetSession(string)
}

func (s coordinatorStores) Session() Store              { return nil }
func (s coordinatorStores) Interrupts() InterruptStore  { return s.interrupts }
func (s coordinatorStores) Transcript() TranscriptStore { return emptyTranscript{} }
func (s coordinatorStores) Runs() RunStore              { return emptyTranscript{} }
func (s coordinatorStores) ReadSnapshot(context.Context, string) (Snapshot, error) {
	if s.snapshotReads != nil {
		*s.snapshotReads++
	}
	return s.snapshot, nil
}
func (s coordinatorStores) ForgetSession(string) {}
func (s coordinatorStores) ApplyFork(_ context.Context, plan ForkPlan) (session.Session, error) {
	if s.forked != nil {
		*s.forked = plan
	}
	return session.Session{ID: "ses_fork"}, nil
}

// The atomic write-sets delegate their interrupt drops to the interrupt fake so
// the coordinator tests observe them (the run-state transition an ApplyTerminal /
// ApplyRollback also commits is verified at the sqlite/bootstrap level).
func (s coordinatorStores) ApplyRollback(ctx context.Context, plan RollbackPlan) error {
	if s.rolledBack != nil {
		*s.rolledBack = plan
	}
	for _, runID := range plan.DropRunIDs {
		_ = s.interrupts.Delete(ctx, plan.SessionID, runID)
	}
	return nil
}
func (s coordinatorStores) ApplyRestore(context.Context, RestorePlan) error { return nil }
func (s coordinatorStores) ApplyDelete(ctx context.Context, plan DeletePlan) error {
	pending, _ := s.interrupts.List(ctx, plan.SessionID)
	for _, p := range pending {
		_ = s.interrupts.Delete(ctx, plan.SessionID, p.RootRunID)
	}
	return nil
}
func (s coordinatorStores) ApplyTerminal(ctx context.Context, plan TerminalPlan) error {
	if s.terminal != nil {
		*s.terminal = plan
	}
	root, ok := plan.RootRun()
	if !ok {
		return errors.New("terminal plan has no root Run")
	}
	return s.interrupts.Delete(ctx, root.SessionID(), root.ID())
}

type coordinatorInterrupts struct {
	pending  map[string]runs.Pending
	deleted  []string
	onDelete func(string)
}

func (s *coordinatorInterrupts) Open(_ context.Context, p runs.Pending) error {
	if s.pending == nil {
		s.pending = map[string]runs.Pending{}
	}
	if _, exists := s.pending[p.RootRunID]; exists {
		return transcript.ErrIdentityConflict
	}
	s.pending[p.RootRunID] = p
	return nil
}

func (s *coordinatorInterrupts) List(_ context.Context, sessionID string) ([]runs.Pending, error) {
	out := make([]runs.Pending, 0, len(s.pending))
	for _, p := range s.pending {
		if sessionID == "" || p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *coordinatorInterrupts) Get(_ context.Context, parentRunID string) (runs.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	return p, ok, nil
}

func (s *coordinatorInterrupts) Consume(_ context.Context, sessionID, parentRunID string) (runs.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	if ok && p.SessionID != sessionID {
		return runs.Pending{}, false, nil
	}
	if ok {
		delete(s.pending, parentRunID)
	}
	return p, ok, nil
}

func (s *coordinatorInterrupts) Delete(_ context.Context, _ string, parentRunID string) error {
	s.deleted = append(s.deleted, parentRunID)
	if s.onDelete != nil {
		s.onDelete(parentRunID)
	}
	delete(s.pending, parentRunID)
	return nil
}

type testClaimer struct {
	claimed  map[string]bool
	released []string
}

func (c *testClaimer) AcquireSession(sessionID string) (func(), bool) {
	if c.claimed == nil {
		c.claimed = map[string]bool{}
	}
	if c.claimed[sessionID] {
		return nil, false
	}
	c.claimed[sessionID] = true
	return func() {
		c.released = append(c.released, sessionID)
		delete(c.claimed, sessionID)
	}, true
}

func (*testClaimer) AcquireWorkingTreeMutation(string) (func(), bool) { return func() {}, true }

func (*testClaimer) ActiveSessions() map[string]bool { return nil }

// newCoordinator builds a Coordinator over test stores and execution release.
func newCoordinator(stores testStores, executions ExecutionReleaser) *Coordinator {
	return newCoordinatorWithAdmissions(stores, executions, new(testClaimer))
}

func newCoordinatorWithAdmissions(stores testStores, executions ExecutionReleaser, admissions Admissions) *Coordinator {
	return New(testDependencies(stores, Dependencies{
		ExecutionReleaser: executions, Paths: testCWDResolver{}, Admissions: admissions,
	}))
}

func testDependencies(stores testStores, deps Dependencies) Dependencies {
	if deps.Admissions == nil {
		deps.Admissions = new(testClaimer)
	}
	deps.Sessions = stores.Session()
	deps.Interrupts = stores.Interrupts()
	deps.Transcript = stores.Transcript()
	deps.Runs = stores.Runs()
	deps.Snapshots = stores
	deps.Writes = stores
	deps.Forgetter = stores
	return deps
}

type testCWDResolver struct {
	resolved string
	err      error
}

func (r testCWDResolver) ResolveExistingDir(path string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.resolved != "" {
		return r.resolved, nil
	}
	return path, nil
}

func (r testCWDResolver) Inspect(path string) (session.WorkspaceIdentity, error) {
	if r.err != nil {
		return session.WorkspaceIdentity{}, r.err
	}
	if r.resolved != "" {
		path = r.resolved
	}
	return session.WorkspaceIdentity{CWD: path, ProjectRoot: path}, nil
}

type emptyTranscript struct{}

func (emptyTranscript) List(context.Context, string) ([]transcript.Item, error) {
	return nil, nil
}

func (emptyTranscript) ListRuns(context.Context, string) ([]run.Run, error) {
	return nil, nil
}
