package sessions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

func TestNewRejectsMalformedDependencies(t *testing.T) {
	var typedNilStore *emptySessionStore
	for _, test := range []struct {
		name string
		deps Dependencies
		want string
	}{
		{name: "empty", deps: Dependencies{}, want: "session store"},
		{name: "typed nil", deps: Dependencies{Sessions: typedNilStore}, want: "session store"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.deps); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}

	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}}}
	deps := testDependencies(stores, Dependencies{
		ExecutionReleaser: inertExecutionReleaser{},
		Paths:             testWorkspaceResolver{},
		MaterialSnapshots: emptyMaterialSnapshotReader{},
	})
	deps.Plan = &PlanServices{}
	if _, err := New(deps); err == nil || !strings.Contains(err.Error(), "plan boundary reader") {
		t.Fatalf("New incomplete Plan error = %v", err)
	}
}

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

func (c coordinatorStores) Session() Store              { return emptySessionStore{} }
func (c coordinatorStores) Interrupts() InterruptStore  { return c.interrupts }
func (c coordinatorStores) Transcript() TranscriptStore { return emptyTranscript{} }
func (c coordinatorStores) Runs() RunStore              { return emptyTranscript{} }
func (c coordinatorStores) ReadSnapshot(context.Context, string) (Snapshot, error) {
	if c.snapshotReads != nil {
		*c.snapshotReads++
	}
	return c.snapshot, nil
}
func (c coordinatorStores) ForgetSession(string) {}
func (c coordinatorStores) ApplyFork(_ context.Context, plan ForkPlan) (session.Session, error) {
	if c.forked != nil {
		*c.forked = plan
	}
	return plan.Child, nil
}

// The atomic write-sets delegate their interrupt drops to the interrupt fake so
// the coordinator tests observe them (the run-state transition an ApplyTerminal /
// ApplyRollback also commits is verified at the sqlite/bootstrap level).
func (c coordinatorStores) ApplyRollback(ctx context.Context, plan RollbackPlan) error {
	if c.rolledBack != nil {
		*c.rolledBack = plan
	}
	for _, runID := range plan.DropRunIDs {
		_ = c.interrupts.Delete(ctx, plan.SessionID, runID)
	}
	return nil
}
func (c coordinatorStores) ApplyRestore(context.Context, RestorePlan) error { return nil }
func (c coordinatorStores) ApplyDelete(ctx context.Context, plan DeletePlan) error {
	pending, _ := c.interrupts.List(ctx, plan.SessionID)
	for _, p := range pending {
		_ = c.interrupts.Delete(ctx, plan.SessionID, p.RootRunID)
	}
	return nil
}
func (c coordinatorStores) ApplyTerminal(ctx context.Context, plan TerminalPlan) error {
	if c.terminal != nil {
		*c.terminal = plan
	}
	root, ok := plan.RootRun()
	if !ok {
		return errors.New("terminal plan has no root Run")
	}
	return c.interrupts.Delete(ctx, root.SessionID(), root.ID())
}

type coordinatorInterrupts struct {
	pending  map[string]runs.Pending
	deleted  []string
	onDelete func(string)
}

func (c *coordinatorInterrupts) Open(_ context.Context, p runs.Pending) error {
	if c.pending == nil {
		c.pending = map[string]runs.Pending{}
	}
	if _, exists := c.pending[p.RootRunID]; exists {
		return transcript.ErrIdentityConflict
	}
	c.pending[p.RootRunID] = p
	return nil
}

func (c *coordinatorInterrupts) List(_ context.Context, sessionID string) ([]runs.Pending, error) {
	out := make([]runs.Pending, 0, len(c.pending))
	for _, p := range c.pending {
		if sessionID == "" || p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *coordinatorInterrupts) Get(_ context.Context, parentRunID string) (runs.Pending, bool, error) {
	p, ok := c.pending[parentRunID]
	return p, ok, nil
}

func (c *coordinatorInterrupts) Consume(_ context.Context, sessionID, parentRunID string) (runs.Pending, bool, error) {
	p, ok := c.pending[parentRunID]
	if ok && p.SessionID != sessionID {
		return runs.Pending{}, false, nil
	}
	if ok {
		delete(c.pending, parentRunID)
	}
	return p, ok, nil
}

func (c *coordinatorInterrupts) Delete(_ context.Context, _ string, parentRunID string) error {
	c.deleted = append(c.deleted, parentRunID)
	if c.onDelete != nil {
		c.onDelete(parentRunID)
	}
	delete(c.pending, parentRunID)
	return nil
}

type testClaimer struct {
	claimed  map[string]bool
	released []string
}

func (t *testClaimer) AcquireSession(sessionID string) (func(), bool) {
	if t.claimed == nil {
		t.claimed = map[string]bool{}
	}
	if t.claimed[sessionID] {
		return nil, false
	}
	t.claimed[sessionID] = true
	return func() {
		t.released = append(t.released, sessionID)
		delete(t.claimed, sessionID)
	}, true
}

func (*testClaimer) AcquireWorkingTreeMutation(string) (func(), bool) { return func() {}, true }

func (*testClaimer) ActiveSessions() map[string]bool { return nil }

// newCoordinator builds a Coordinator over test stores and execution release.
func newCoordinator(stores testStores, executions ExecutionReleaser) *Coordinator {
	return newCoordinatorWithAdmissions(stores, executions, new(testClaimer))
}

func newCoordinatorWithAdmissions(stores testStores, executions ExecutionReleaser, admissions Admissions) *Coordinator {
	return mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: executions, Paths: testWorkspaceResolver{}, Admissions: admissions,
	}))
}

func testDependencies(stores testStores, deps Dependencies) Dependencies {
	if deps.Admissions == nil {
		deps.Admissions = new(testClaimer)
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Unix(2, 0).UTC() }
	}
	if deps.NewID == nil {
		deps.NewID = func() string { return "ses_fork" }
	}
	var runSequence, itemSequence int
	if deps.NewRunID == nil {
		deps.NewRunID = func() string {
			runSequence++
			return fmt.Sprintf("run_fork_%d", runSequence)
		}
	}
	if deps.NewItemID == nil {
		deps.NewItemID = func() string {
			itemSequence++
			return fmt.Sprintf("item_fork_%d", itemSequence)
		}
	}
	if deps.NewToolResultID == nil {
		deps.NewToolResultID = toolresult.NewID
	}
	if !deps.DefaultModelSelection.Configured() {
		deps.DefaultModelSelection, _ = modelref.New("test-provider", "test-model")
	}
	if deps.Models == nil {
		deps.Models = testModelAdmitter{}
	}
	deps.Sessions = stores.Session()
	deps.Interrupts = stores.Interrupts()
	deps.Transcript = stores.Transcript()
	deps.Runs = stores.Runs()
	deps.Snapshots = stores
	if material, ok := stores.(MaterialSnapshotReader); ok {
		deps.MaterialSnapshots = material
	}
	deps.Writes = stores
	deps.Forgetter = stores
	return deps
}

func mustNewCoordinator(deps Dependencies) *Coordinator {
	defaults := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}}}
	if deps.Sessions == nil {
		deps.Sessions = defaults.Session()
	}
	if deps.Interrupts == nil {
		deps.Interrupts = defaults.Interrupts()
	}
	if deps.Transcript == nil {
		deps.Transcript = defaults.Transcript()
	}
	if deps.Runs == nil {
		deps.Runs = defaults.Runs()
	}
	if deps.Snapshots == nil {
		deps.Snapshots = defaults
	}
	if deps.MaterialSnapshots == nil {
		deps.MaterialSnapshots = emptyMaterialSnapshotReader{}
	}
	if deps.Writes == nil {
		deps.Writes = defaults
	}
	if deps.Forgetter == nil {
		deps.Forgetter = defaults
	}
	if deps.ExecutionReleaser == nil {
		deps.ExecutionReleaser = inertExecutionReleaser{}
	}
	if deps.Paths == nil {
		deps.Paths = testWorkspaceResolver{}
	}
	if deps.Admissions == nil {
		deps.Admissions = new(testClaimer)
	}
	if deps.NewID == nil {
		deps.NewID = func() string { return "ses_test" }
	}
	if deps.NewRunID == nil {
		deps.NewRunID = func() string { return "run_test" }
	}
	if deps.NewItemID == nil {
		deps.NewItemID = func() string { return "item_test" }
	}
	if deps.NewToolResultID == nil {
		deps.NewToolResultID = toolresult.NewID
	}
	if !deps.DefaultModelSelection.Configured() {
		deps.DefaultModelSelection, _ = modelref.New("test-provider", "test-model")
	}
	if deps.Models == nil {
		deps.Models = testModelAdmitter{}
	}
	coordinator, err := New(deps)
	if err != nil {
		panic(err)
	}
	return coordinator
}

func mustTestSelection(t *testing.T, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New: %v", err)
	}
	return selection
}

type inertExecutionReleaser struct{}

func (inertExecutionReleaser) Release(context.Context, runs.ExecutorRef) error { return nil }

type testModelAdmitter struct{}

func (testModelAdmitter) AdmitSelection(modelref.Selection) error { return nil }

type emptyMaterialSnapshotReader struct{}

func (emptyMaterialSnapshotReader) ReadMaterialSnapshot(context.Context, string) (MaterialSnapshot, error) {
	return MaterialSnapshot{}, nil
}

type emptySessionStore struct{}

func (emptySessionStore) List(context.Context) ([]session.Session, error) { return nil, nil }
func (emptySessionStore) ListPage(context.Context, bool, int64, string, int) ([]session.Session, error) {
	return nil, nil
}
func (emptySessionStore) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, session.ErrNotFound
}
func (emptySessionStore) Insert(context.Context, session.Session) error { return nil }
func (emptySessionStore) Save(context.Context, uint64, session.Session) error {
	return nil
}

type testWorkspaceResolver struct {
	resolved string
	missing  bool
	err      error
}

func (t testWorkspaceResolver) ResolveExistingDir(path string) (string, error) {
	if t.err != nil {
		return "", t.err
	}
	if t.resolved != "" {
		return t.resolved, nil
	}
	return path, nil
}

func (t testWorkspaceResolver) Inspect(path string) (workspace.Resolved, error) {
	if t.err != nil {
		return workspace.Resolved{}, t.err
	}
	if t.resolved != "" {
		path = t.resolved
	}
	return workspace.Resolved{Path: path, ProjectRoot: path, Missing: t.missing}, nil
}

type emptyTranscript struct{}

func (emptyTranscript) List(context.Context, string) ([]transcript.Item, error) {
	return nil, nil
}

func (emptyTranscript) ListRuns(context.Context, string) ([]run.Run, error) {
	return nil, nil
}

func (emptyTranscript) ListNonTerminalRuns(context.Context) ([]run.Run, error) {
	return nil, nil
}
