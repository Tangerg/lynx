package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

// testRuntime is the Delivery test seam newTestServer uses to build the Run
// coordinator: semantic execution plus a Run-segment effects factory. The test
// double stays on application-owned ports so handler fixtures do not depend on
// Agent adapter handles.
type testRuntime interface {
	runs.RootExecutionStarter
	runs.ExecutionObserver
	runs.ExecutionReleaser
	runs.ContinuationExecutor
	runs.ExecutionSteerer
	runs.RunningSubtreeCanceler
	runs.WaitingSubtreeCancellationPreparer
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
}

func serverPending(
	runID, sessionID, executorID, processID string,
	open []transcript.Interrupt,
	createdAt time.Time,
) runs.Pending {
	if executorID == "" {
		executorID = "exec_" + runID
	}
	if processID == "" {
		processID = "process_" + runID
	}
	if createdAt.IsZero() {
		createdAt = time.Unix(1, 0).UTC()
	}
	if len(open) == 0 {
		open = []transcript.Interrupt{{
			ItemID: "interrupt_" + runID, ItemOccurredAt: createdAt,
			RunID:    runID,
			Kind:     interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}}
	} else {
		open = append([]transcript.Interrupt(nil), open...)
		for index := range open {
			if open[index].RunID == "" {
				open[index].RunID = runID
			}
			if open[index].ItemOccurredAt.IsZero() {
				open[index].ItemOccurredAt = createdAt
			}
		}
	}
	bindings := make([]runs.SuspensionBinding, len(open))
	capabilities := run.RunCapabilities{}
	for index, interrupt := range open {
		capabilities.InterruptKinds = append(capabilities.InterruptKinds, interrupt.Kind)
		bindings[index] = runs.SuspensionBinding{
			InterruptItemID: interrupt.ItemID,
			MemberID:        processID,
			SuspensionID:    fmt.Sprintf("suspension_%s_%d", processID, index),
		}
	}
	return runs.Pending{
		RootRunID:    runID,
		SessionID:    sessionID,
		ExecutorID:   executorID,
		Interrupts:   open,
		Suspensions:  bindings,
		Capabilities: capabilities.Normalized(),
		Continuations: []runs.Continuation{{
			RunID:        runID,
			MemberID:     processID,
			RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt,
	}
}

// executionRuntime is the combined application-owned execution surface this
// integration harness supplies. Production consumers still receive the two
// narrower interfaces independently.
type executionRuntime interface {
	runs.RootExecutionStarter
	runs.ExecutionObserver
	runs.ExecutionReleaser
	runs.ContinuationExecutor
	runs.ExecutionSteerer
	runs.RunningSubtreeCanceler
	runs.WaitingSubtreeCancellationPreparer
}

// stubRuntime is the delivery session/lifecycle test double: it provides the run
// executor + effects (testRuntime) over its own in-memory + sqlite stores, and
// the coordinator-provider seams for Sessions, queries, and execution control.
type stubRuntime struct {
	sess        *sqlite.SessionStore
	model       string
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        *sqlite.TranscriptStore   // durable Item history
	runs        *sqlite.RunStore          // durable Run records (rollback/fork read runs)
	toolResults *sqlite.ToolResultStore
	plan        *sqlite.PlanStore                   // session-scoped state: exported, restored, dropped with the session
	interrupts  *persistence.InterruptStore         // open-interrupt registry (rollback clears dropped)
	muts        *persistence.WorkspaceMutationStore // §8.5 recoverable file-rollback log
	execution   executionRuntime
	admissions  *admission.Gate
}

// sessionsCoordinatorProvider is the optional test seam newTestServer uses to
// wire s.sessions: a fake that can build the real lifecycle coordinator over its
// own in-memory stores (stubRuntime). Fakes that never drive a lifecycle
// write-set may omit it, leaving s.sessions nil.
type sessionsCoordinatorProvider interface {
	sessionsCoordinator(admissions sessions.Admissions) *sessions.Coordinator
}

// queriesCoordinatorProvider is the parallel seam for the read coordinator: a
// fake that can build it over its own transcript and interrupt stores. Fakes
// that never drive a read (live-run tests) may omit it.
type queriesCoordinatorProvider interface {
	queriesCoordinator() *queries.Coordinator
}

// runProjectionProvider is the seam for the durable run read that addressing a
// live segment resolves through (subscribe / steer). Fakes with no run store may
// omit it; a handler that then addresses a segment fails loudly rather than
// guessing what the run is doing.
type runProjectionProvider interface {
	runProjection() runs.RunProjection
}

func (s stubRuntime) runProjection() runs.RunProjection {
	if s.runs == nil {
		return nil
	}
	return s.runs
}

func (s stubRuntime) queriesCoordinator() *queries.Coordinator {
	return queries.New(queries.Dependencies{
		Transcript: s.hist,
		Interrupts: s.interrupts,
		Runs:       s.runs,
		Sessions:   s.sess,
		// The composition root passes the same store to both the write path and the
		// query one, because features.plan IS "the store exists" — so a harness that
		// wired only the writes could not reach plan.get at all.
		Plan: s.plan,
	})
}

func newTestServer(rt testRuntime) *Server {
	s := &Server{}
	admissions := &admission.Gate{}
	var sessionPorts runs.SessionPorts
	// Wire the session/run lifecycle coordinator over the fake's in-memory stores
	// when the fake provides one, mirroring the composition root.
	if p, ok := rt.(sessionsCoordinatorProvider); ok {
		sessionCoordinator := p.sessionsCoordinator(admissions)
		s.sessions = sessionCoordinator
		sessionPorts = runs.SessionPorts{
			Reader:       sessionCoordinator,
			Creator:      sessionCoordinator,
			ActiveRuns:   sessionCoordinator,
			Interrupts:   sessionCoordinator,
			Terminations: sessionCoordinator,
		}
	}
	var ids atomic.Uint64
	var runProjection runs.RunProjection
	if p, ok := rt.(runProjectionProvider); ok {
		runProjection = p.runProjection()
	}
	projectionWriter := rt.RunSegmentEffects(nil, nil)
	s.runs = runs.NewCoordinator(runs.Dependencies{
		RootStarts:   rt,
		Observations: rt,
		Releases:     rt,
		Continuation: rt,
		Steering:     rt,
		RunningTrees: rt,
		WaitingTrees: rt,
		Session:      sessionPorts,
		Projection: runs.ProjectionPorts{
			Openings:     projectionWriter,
			Events:       projectionWriter,
			Barriers:     projectionWriter,
			WaitingEdits: projectionWriter,
			Workspace:    projectionWriter,
			Finalizer:    projectionWriter,
		},
		Runs:       runProjection,
		Admissions: admissions,
		Now:        time.Now,
		NewRunID: func() string {
			return fmt.Sprintf("run_test_%d", ids.Add(1))
		},
		NewSegmentID: func() string {
			return fmt.Sprintf("seg_test_%d", ids.Add(1))
		},
	})
	if p, ok := rt.(queriesCoordinatorProvider); ok {
		s.queries = p.queriesCoordinator()
	}
	// Capability handler tests replace this coordinator through serverWithModels;
	// session projection reads its complete model choice from the session use case.
	s.models = models.New(models.Config{})
	// Default to a disabled schedules coordinator (schedules.* report
	// capability_not_negotiated); schedule tests replace it with a fake registry.
	s.schedules = schedules.New(schedules.Dependencies{})
	return s
}

// serverWithModels builds a Server whose only wired coordinator is the models one
// — enough for the providers / models handler tests.
func serverWithModels(cfg models.Config) *Server {
	return &Server{models: models.New(cfg)}
}

// serverWithTools builds a Server whose only wired coordinator is the tools one —
// enough for the tools.* handler tests.
func serverWithTools(useCases toolUseCases) *Server {
	return &Server{tools: useCases}
}

func (s stubRuntime) Transcript() *sqlite.TranscriptStore     { return s.hist }
func (s stubRuntime) Interrupts() *persistence.InterruptStore { return s.interrupts }

// MessageCount and TruncateMessages operate on the in-memory history map,
// mirroring the engine's conversation-history store closely enough for
// rollback/fork tests.
func (s stubRuntime) MessageCount(_ context.Context, id string) (int, error) {
	return len(s.history[id]), nil
}

// RunInTx in the stub just runs fn; the in-memory stub has no real transaction,
// while production wires the sqlite-backed transactor.
func (s stubRuntime) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (s stubRuntime) TruncateMessages(_ context.Context, id string, keepN int) error {
	msgs := s.history[id]
	if keepN <= 0 {
		delete(s.history, id)
		return nil
	}
	if keepN < len(msgs) {
		s.history[id] = msgs[:keepN]
	}
	return nil
}

// executionStub supplies the default inert executor through application-owned
// values. Most Session tests never drive it, but complete methods keep an
// accidental call deterministic instead of dispatching through a nil embed.
type executionStub struct{}

func (executionStub) Observe(context.Context, runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	return func(func(runs.ExecutorEvent) bool) {}, nil
}
func (executionStub) ValidateRootStart(req runs.RootExecutionStart) error { return req.Validate() }
func (executionStub) StageRoot(_ context.Context, req runs.RootExecutionStart) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{SessionID: req.SessionID, ExecutorID: "exec_test"}, nil
}
func (executionStub) BeginRoot(context.Context, runs.ExecutorRef) error { return nil }
func (executionStub) ClaimWaiting(_ context.Context, ref runs.ExecutorRef) (runs.ExecutorRef, error) {
	return ref, nil
}
func (executionStub) ResumeWaiting(context.Context, runs.ExecutorRef, []runs.SuspensionAnswer, []interrupt.Kind) error {
	return nil
}
func (executionStub) RestoreWaiting(_ context.Context, req runs.RehydrateExecution) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{SessionID: req.SessionID, ExecutorID: req.ExecutorID}, nil
}
func (executionStub) Release(context.Context, runs.ExecutorRef) error { return nil }
func (executionStub) CancelRunningSubtree(context.Context, runs.ExecutorRef, string) error {
	return nil
}
func (executionStub) PrepareWaitingSubtreeCancellation(
	context.Context,
	runs.ExecutorRef,
	string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	return runs.PreparedWaitingSubtreeCancellation{}, errors.New("test execution: waiting subtree cancellation is unavailable")
}
func (executionStub) Steer(context.Context, runs.ExecutorRef, []transcript.ContentBlock) error {
	return nil
}

type recordingExecutions struct {
	executionStub
	canceled []runs.ExecutorRef
}

func (r *recordingExecutions) Release(_ context.Context, ref runs.ExecutorRef) error {
	r.canceled = append(r.canceled, ref)
	return nil
}

func (s stubRuntime) executionController() executionRuntime {
	if s.execution != nil {
		return s.execution
	}
	return executionStub{}
}

func (s stubRuntime) Observe(ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	return s.executionController().Observe(ctx, ref)
}

func (s stubRuntime) ValidateRootStart(req runs.RootExecutionStart) error {
	return s.executionController().ValidateRootStart(req)
}

func (s stubRuntime) StageRoot(ctx context.Context, req runs.RootExecutionStart) (runs.ExecutorRef, error) {
	return s.executionController().StageRoot(ctx, req)
}

func (s stubRuntime) BeginRoot(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().BeginRoot(ctx, ref)
}

func (s stubRuntime) ClaimWaiting(ctx context.Context, ref runs.ExecutorRef) (runs.ExecutorRef, error) {
	return s.executionController().ClaimWaiting(ctx, ref)
}

func (s stubRuntime) ResumeWaiting(ctx context.Context, prepared runs.ExecutorRef, answers []runs.SuspensionAnswer, interruptKinds []interrupt.Kind) error {
	return s.executionController().ResumeWaiting(ctx, prepared, answers, interruptKinds)
}

func (s stubRuntime) RestoreWaiting(ctx context.Context, req runs.RehydrateExecution) (runs.ExecutorRef, error) {
	return s.executionController().RestoreWaiting(ctx, req)
}

func (s stubRuntime) Cancel(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().Release(ctx, ref)
}

func (s stubRuntime) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	processID string,
) error {
	return s.executionController().CancelRunningSubtree(ctx, ref, processID)
}

func (s stubRuntime) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	ref runs.ExecutorRef,
	processID string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	return s.executionController().PrepareWaitingSubtreeCancellation(
		ctx,
		ref,
		processID,
	)
}

func (s stubRuntime) Steer(ctx context.Context, ref runs.ExecutorRef, input []transcript.ContentBlock) error {
	return s.executionController().Steer(ctx, ref, input)
}

func (s stubRuntime) Release(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().Release(ctx, ref)
}

type stubExecutionReleaser struct {
	rt *stubRuntime
}

func (t stubExecutionReleaser) Release(ctx context.Context, ref runs.ExecutorRef) error {
	return t.rt.Release(ctx, runs.ExecutorRef{SessionID: ref.SessionID, ExecutorID: ref.ExecutorID})
}

type stubLifecycleStores struct {
	rt *stubRuntime
}

func (s stubLifecycleStores) Session() sessions.Store { return s.rt.sess }

func (s stubLifecycleStores) Interrupts() sessions.InterruptStore { return s.rt.interrupts }

func (s stubLifecycleStores) Transcript() sessions.TranscriptStore { return s.rt.hist }

func (s stubLifecycleStores) Runs() sessions.RunStore { return s.rt.runs }

func (s stubLifecycleStores) ReadSnapshot(ctx context.Context, id string) (sessions.Snapshot, error) {
	ses, err := s.rt.sess.Get(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	items, err := s.rt.hist.List(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	runs, err := s.rt.runs.ListRuns(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	messages, err := s.rt.ReadHistory(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	var toolResults []toolresult.Blob
	if s.rt.toolResults != nil {
		toolResults, err = s.rt.toolResults.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	var steps []plan.Step
	if s.rt.plan != nil {
		steps, err = s.rt.plan.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	return sessions.Snapshot{
		Session: ses, Messages: messages, Items: items, Runs: runs,
		ToolResults: toolResults, Plan: steps,
	}, nil
}

func (s stubLifecycleStores) ApplyFork(ctx context.Context, plan sessions.ForkPlan) (session.Session, error) {
	child, err := s.rt.sess.Fork(ctx, plan.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	if err := s.rt.SeedHistory(ctx, child.ID, plan.Messages); err != nil {
		return session.Session{}, err
	}
	if plan.Title != "" {
		if err := s.rt.sess.Rename(ctx, child.ID, plan.Title); err != nil {
			return session.Session{}, err
		}
		child.Title = plan.Title
	}
	return child, nil
}

// The atomic write-sets over the stub's in-memory chat log + real sqlite
// transcript/run/interrupt/session stores, mirroring the persistence adapter.
func (s stubLifecycleStores) ApplyRollback(ctx context.Context, plan sessions.RollbackPlan) error {
	if plan.KeepMark >= 0 {
		if err := s.rt.TruncateMessages(ctx, plan.SessionID, plan.KeepMark); err != nil {
			return err
		}
	}
	for _, runID := range plan.DropRunIDs {
		if err := s.rt.hist.DeleteRun(ctx, plan.SessionID, runID); err != nil {
			return err
		}
		if err := s.rt.runs.Delete(ctx, plan.SessionID, runID); err != nil {
			return err
		}
		if err := s.rt.interrupts.Delete(ctx, plan.SessionID, runID); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) ApplyRestore(ctx context.Context, plan sessions.RestorePlan) error {
	id := plan.Session.ID
	if err := s.rt.sess.Restore(ctx, plan.Session); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, id); err != nil {
		return err
	}
	if err := s.rt.hist.DeleteSession(ctx, id); err != nil {
		return err
	}
	if err := s.rt.runs.DeleteForSession(ctx, id); err != nil {
		return err
	}
	if s.rt.toolResults != nil {
		if err := s.rt.toolResults.DropSession(ctx, id); err != nil {
			return err
		}
	}
	if err := s.rt.TruncateMessages(ctx, id, 0); err != nil {
		return err
	}
	if err := s.rt.SeedHistory(ctx, id, plan.Messages); err != nil {
		return err
	}
	for _, r := range plan.Runs {
		if err := s.rt.runs.Restore(ctx, r); err != nil {
			return err
		}
	}
	for _, it := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, it); err != nil {
			return err
		}
	}
	for _, blob := range plan.ToolResults {
		if s.rt.toolResults == nil {
			return errors.New("test runtime: tool-result persistence is unavailable")
		}
		if err := s.rt.toolResults.Restore(ctx, blob); err != nil {
			return err
		}
	}
	// Replaced, never deleted-and-reinserted: the revision has to come out greater
	// than what this session already published.
	if s.rt.plan != nil {
		if err := s.rt.plan.Replace(ctx, id, plan.Plan); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	return s.deleteSession(ctx, plan.SessionID)
}

func (s stubLifecycleStores) deleteSession(ctx context.Context, sessionID string) error {
	if err := s.rt.hist.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rt.runs.DeleteForSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rt.TruncateMessages(ctx, sessionID, 0); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, sessionID); err != nil {
		return err
	}
	if s.rt.toolResults != nil {
		if err := s.rt.toolResults.DropSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return s.rt.sess.Delete(ctx, sessionID)
}

func (s stubLifecycleStores) ApplyTerminal(ctx context.Context, plan sessions.TerminalPlan) error {
	root, ok := plan.RootRun()
	if !ok {
		return errors.New("terminal plan has no root Run")
	}
	for _, item := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, item); err != nil {
			return err
		}
	}
	if err := s.rt.interrupts.Delete(ctx, root.SessionID, root.ID); err != nil {
		return err
	}
	for _, record := range plan.Runs {
		if record.Outcome != nil && *record.Outcome == run.OutcomeLost {
			if err := s.rt.runs.RecoverLost(ctx, record); err != nil {
				return err
			}
			continue
		}
		if err := s.rt.runs.Terminalize(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.rt.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.rt.interrupts.Delete(ctx, sessionID, p.RootRunID); err != nil {
			return err
		}
	}
	return nil
}

type stubMessageCounter struct{ rt stubRuntime }

func (s stubMessageCounter) Count(ctx context.Context, id string) (int, error) {
	return s.rt.MessageCount(ctx, id)
}

type stubTitleGenerator struct{}

func (stubTitleGenerator) Generate(context.Context, string) (string, error) { return "", nil }

// sessionsCoordinator builds the real lifecycle coordinator over the stub's
// in-memory stores and execution release, so newTestServer can wire s.sessions the way the
// composition root does — delivery drives every lifecycle write-set through it.
// File restore stays disabled (nil restorer); the checkpoint tests rebuild it
// with a real restorer via [stubRuntime.sessionsCoordinatorWithRestorer].
func (s *stubRuntime) sessionsCoordinator(admissions sessions.Admissions) *sessions.Coordinator {
	gate, ok := admissions.(*admission.Gate)
	if !ok {
		panic("test runtime requires admission.Gate")
	}
	s.admissions = gate
	return s.sessionsCoordinatorWithRestorer(nil, admissions)
}

func (s *stubRuntime) sessionsCoordinatorWithRestorer(checkpoints sessions.WorkspaceCheckpoints, shared ...sessions.Admissions) *sessions.Coordinator {
	admissions := sessions.Admissions(&admission.Gate{})
	if len(shared) > 0 && shared[0] != nil {
		admissions = shared[0]
	}
	stores := stubLifecycleStores{rt: s}
	return sessions.New(sessions.Dependencies{
		Sessions:          s.sess,
		Interrupts:        s.interrupts,
		Transcript:        s.hist,
		Runs:              s.runs,
		Snapshots:         stores,
		Writes:            stores,
		Forgetter:         s,
		ExecutionReleaser: stubExecutionReleaser{rt: s},
		Paths:             workspacepath.Resolver{},
		Checkpoints:       checkpoints,
		Mutations:         s.muts,
		Admissions:        admissions,
	})
}

func (s stubRuntime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Interrupts:         s.interrupts,
		Sessions:           s.sess,
		Transcript:         s.hist,
		ToolResults:        s.toolResults,
		Messages:           stubMessageCounter{rt: s},
		Titles:             stubTitleGenerator{},
		RunState:           s.runWriter(),
		Tx:                 s.RunInTx,
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
}

// runWriter is the real Run table when the fixture has one, so a committed
// terminal actually lands where every Run read comes from. Fixtures that only
// exercise streaming keep the no-op.
func (s stubRuntime) runWriter() runsegment.RunWriter {
	if s.runs != nil {
		return s.runs
	}
	return stubRunState{}
}

type stubRunState struct{}

func (stubRunState) Admit(context.Context, run.RunDraft) error { return nil }
func (stubRunState) Resume(
	context.Context,
	string,
	run.RunResumeDraft,
	time.Time,
) error {
	return nil
}
func (stubRunState) Suspend(context.Context, transcript.Run) error     { return nil }
func (stubRunState) Terminalize(context.Context, transcript.Run) error { return nil }

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate; these tests have no live execution state to forget.
func (stubRuntime) ForgetSession(string) {}

func (s stubRuntime) ReadHistory(_ context.Context, id string) ([]chat.Message, error) {
	return s.history[id], nil
}
func (s stubRuntime) SeedHistory(_ context.Context, id string, msgs []chat.Message) error {
	if s.history != nil {
		s.history[id] = append(s.history[id], msgs...)
	}
	return nil
}

func newSessionServer(t *testing.T) (*Server, *sqlite.SessionStore, *stubRuntime) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	// Interrupts is always wired in production (runtime composition root), and
	// the wire status now reads it (liveStatus), so give the stub a real store.
	runtime := &stubRuntime{sess: svc, model: "default-model", interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db))}
	return newTestServer(runtime), svc, runtime
}
