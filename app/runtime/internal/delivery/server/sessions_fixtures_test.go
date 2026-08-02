package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

// testRuntime is the delivery test seam newTestServer builds the run coordinator
// from: the executor + the run-segment effects factory. Production wires the
// agentexec turn executor + a Host-built effects; the stub provides both, plus
// the optional coordinator-provider seams asserted below.
type testRuntime interface {
	runs.SegmentExecutor
	runs.TurnControl
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
}

func serverPending(
	runID, sessionID, turnID, processID string,
	open []transcript.Interrupt,
	createdAt time.Time,
) interrupts.Pending {
	if turnID == "" {
		turnID = "turn_" + runID
	}
	if processID == "" {
		processID = "process_" + runID
	}
	if createdAt.IsZero() {
		createdAt = time.Unix(1, 0).UTC()
	}
	if len(open) == 0 {
		open = []transcript.Interrupt{{
			ItemID:   "interrupt_" + runID,
			RunID:    runID,
			Kind:     execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}}
	} else {
		open = append([]transcript.Interrupt(nil), open...)
		for index := range open {
			if open[index].RunID == "" {
				open[index].RunID = runID
			}
		}
	}
	bindings := make([]interrupts.SuspensionBinding, len(open))
	profile := execution.RunProtocolProfile{}
	for index, interrupt := range open {
		profile.InterruptKinds = append(profile.InterruptKinds, interrupt.Kind)
		bindings[index] = interrupts.SuspensionBinding{
			InterruptItemID: interrupt.ItemID,
			ProcessID:       processID,
			SuspensionID:    fmt.Sprintf("suspension_%s_%d", processID, index),
		}
	}
	return interrupts.Pending{
		RootRunID:       runID,
		SessionID:       sessionID,
		TurnID:          turnID,
		Interrupts:      open,
		Suspensions:     bindings,
		ProtocolProfile: profile.Normalized(),
		Continuations: []interrupts.Continuation{{
			RunID:        runID,
			ProcessID:    processID,
			RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt,
	}
}

// turnRuntime is this integration harness's complete executor view. Production
// consumers do not share this surface: turn.Executor, session cleanup, and
// run-segment persistence each declare their own smaller ports.
type turnRuntime interface {
	Events(context.Context, turn.TurnHandle) (iter.Seq[runs.ExecutorEvent], error)
	InjectSteering(context.Context, turn.TurnHandle, []transcript.ContentBlock) error
	PrepareTurn(context.Context, runs.StartTurn) (turn.TurnHandle, error)
	ActivateTurn(context.Context, turn.TurnHandle) error
	Resume(context.Context, turn.TurnHandle, []agentexec.SuspensionAnswer, []execution.InterruptKind) error
	ProcessID(context.Context, turn.TurnHandle) (string, error)
	Rehydrate(context.Context, runs.RehydrateTurn) (turn.TurnHandle, error)
	Cancel(context.Context, turn.TurnHandle) error
	CancelSubtree(context.Context, turn.TurnHandle, string) error
}

// stubRuntime is the delivery session/lifecycle test double: it provides the run
// executor + effects (testRuntime) over its own in-memory + sqlite stores, and
// the coordinator-provider seams (sessions / queries / turn control).
type stubRuntime struct {
	sess        *sqlite.SessionStore
	model       string
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        *sqlite.TranscriptStore   // durable Item history
	runs        *sqlite.RunStore          // durable Run records (rollback/fork read runs)
	toolResults *sqlite.ToolResultStore
	todos       *sqlite.TodoStore              // session-scoped state: exported, restored, dropped with the session
	interrupts  *sqlite.InterruptStore         // open-interrupt registry (rollback clears dropped)
	muts        *sqlite.WorkspaceMutationStore // §8.5 recoverable file-rollback log
	turns       turnRuntime
	admissions  *admission.Gate
}

// sessionsCoordinatorProvider is the optional test seam newTestServer uses to
// wire s.sessions: a fake that can build the real lifecycle coordinator over its
// own in-memory stores (stubRuntime). Fakes that never drive a lifecycle
// write-set may omit it, leaving s.sessions nil.
type sessionsCoordinatorProvider interface {
	sessionsCoordinator(admissions sessions.SessionAdmissions) *sessions.Coordinator
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
		// query one, because features.todos IS "the store exists" — so a harness that
		// wired only the writes could not reach todos.get at all.
		Todos: s.todos,
	})
}

func newTestServer(rt testRuntime) *Server {
	s := &Server{}
	admissions := &admission.Gate{}
	var lifecycle runs.SessionLifecycle
	// Wire the session/run lifecycle coordinator over the fake's in-memory stores
	// when the fake provides one, mirroring the composition root.
	if p, ok := rt.(sessionsCoordinatorProvider); ok {
		s.sessions = p.sessionsCoordinator(admissions)
		lifecycle = s.sessions.(runs.SessionLifecycle)
	}
	var ids atomic.Uint64
	var runProjection runs.RunProjection
	if p, ok := rt.(runProjectionProvider); ok {
		runProjection = p.runProjection()
	}
	s.coordinator = runs.NewCoordinator(runs.Dependencies{
		Segments:   rt,
		Turns:      rt,
		Sessions:   lifecycle,
		Effects:    rt.RunSegmentEffects(nil, nil),
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

func (s stubRuntime) Transcript() *sqlite.TranscriptStore { return s.hist }
func (s stubRuntime) Interrupts() *sqlite.InterruptStore  { return s.interrupts }

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

// turnStub supplies the default inert executor. Most session tests never drive
// a turn, so no method is implemented unless a specific case needs it.
type turnStub struct{ turnRuntime }

func (turnStub) Cancel(context.Context, turn.TurnHandle) error { return nil }

type recordingTurns struct {
	turnRuntime
	canceled []turn.TurnHandle
}

func (r *recordingTurns) Cancel(_ context.Context, h turn.TurnHandle) error {
	r.canceled = append(r.canceled, h)
	return nil
}

func (s stubRuntime) turnDispatcher() turnRuntime {
	if s.turns != nil {
		return s.turns
	}
	return turnStub{}
}

func (s stubRuntime) TurnEvents(ctx context.Context, ref execution.TurnRef) (iter.Seq[runs.ExecutorEvent], error) {
	return turn.NewExecutor(s.turnDispatcher()).TurnEvents(ctx, ref)
}

func (s stubRuntime) ValidateStart(req runs.StartTurn) error {
	return turn.NewExecutor(s.turnDispatcher()).ValidateStart(req)
}

func (s stubRuntime) PrepareStart(ctx context.Context, req runs.StartTurn) (execution.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).PrepareStart(ctx, req)
}

func (s stubRuntime) Activate(ctx context.Context, ref execution.TurnRef) error {
	return turn.NewExecutor(s.turnDispatcher()).Activate(ctx, ref)
}

func (s stubRuntime) Prepare(ctx context.Context, ref execution.TurnRef) (execution.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).Prepare(ctx, ref)
}

func (s stubRuntime) Resume(ctx context.Context, prepared execution.TurnRef, answers []interrupts.SuspensionAnswer, interruptKinds []execution.InterruptKind) error {
	return turn.NewExecutor(s.turnDispatcher()).Resume(ctx, prepared, answers, interruptKinds)
}

func (s stubRuntime) Rehydrate(ctx context.Context, req runs.RehydrateTurn) (execution.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).Rehydrate(ctx, req)
}

func (s stubRuntime) Cancel(ctx context.Context, ref execution.TurnRef) error {
	return turn.NewExecutor(s.turnDispatcher()).CancelTurn(ctx, ref)
}

func (s stubRuntime) CancelSubtree(
	ctx context.Context,
	ref execution.TurnRef,
	processID string,
) error {
	return turn.NewExecutor(s.turnDispatcher()).CancelSubtree(ctx, ref, processID)
}

func (s stubRuntime) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	ref execution.TurnRef,
	processID string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	return turn.NewExecutor(s.turnDispatcher()).PrepareWaitingSubtreeCancellation(
		ctx,
		ref,
		processID,
	)
}

func (s stubRuntime) Steer(ctx context.Context, ref execution.TurnRef, input []transcript.ContentBlock) error {
	return turn.NewExecutor(s.turnDispatcher()).Steer(ctx, ref, input)
}

func (s stubRuntime) CancelTurn(ctx context.Context, ref execution.TurnRef) error {
	return s.turnDispatcher().Cancel(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

func (s stubRuntime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return s.turnDispatcher().ProcessID(ctx, handle)
}

type stubLifecycleTurns struct {
	rt *stubRuntime
}

func (t stubLifecycleTurns) Cancel(ctx context.Context, ref execution.TurnRef) error {
	return t.rt.CancelTurn(ctx, execution.TurnRef{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

type stubLifecycleStores struct {
	rt *stubRuntime
}

func (s stubLifecycleStores) Session() sessions.SessionStore { return s.rt.sess }

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
	var toolResults []offload.ToolResultBlob
	if s.rt.toolResults != nil {
		toolResults, err = s.rt.toolResults.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	var todos []todo.Item
	if s.rt.todos != nil {
		todos, err = s.rt.todos.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	return sessions.Snapshot{
		Session: ses, Messages: messages, Items: items, Runs: runs,
		ToolResults: toolResults, Todos: todos,
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
	if s.rt.todos != nil {
		if err := s.rt.todos.Replace(ctx, id, plan.Todos); err != nil {
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
	for _, run := range plan.Runs {
		if run.Outcome != nil && *run.Outcome == execution.OutcomeError {
			if err := s.rt.runs.RecoverLost(ctx, run); err != nil {
				return err
			}
			continue
		}
		if err := s.rt.runs.Terminalize(ctx, run); err != nil {
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
// in-memory stores and turns, so newTestServer can wire s.sessions the way the
// composition root does — delivery drives every lifecycle write-set through it.
// File restore stays disabled (nil restorer); the checkpoint tests rebuild it
// with a real restorer via [stubRuntime.sessionsCoordinatorWithRestorer].
func (s *stubRuntime) sessionsCoordinator(admissions sessions.SessionAdmissions) *sessions.Coordinator {
	gate, ok := admissions.(*admission.Gate)
	if !ok {
		panic("test runtime requires admission.Gate")
	}
	s.admissions = gate
	return s.sessionsCoordinatorWithRestorer(nil, admissions)
}

func (s *stubRuntime) sessionsCoordinatorWithRestorer(checkpoints sessions.WorkspaceCheckpoints, shared ...sessions.SessionAdmissions) *sessions.Coordinator {
	admissions := sessions.SessionAdmissions(&admission.Gate{})
	if len(shared) > 0 && shared[0] != nil {
		admissions = shared[0]
	}
	stores := stubLifecycleStores{rt: s}
	return sessions.New(sessions.Dependencies{
		Sessions:    s.sess,
		Interrupts:  s.interrupts,
		Transcript:  s.hist,
		Runs:        s.runs,
		Snapshots:   stores,
		Writes:      stores,
		Forgetter:   s,
		Turns:       stubLifecycleTurns{rt: s},
		Paths:       workspacepath.Resolver{},
		Checkpoints: checkpoints,
		Mutations:   s.muts,
		Admissions:  admissions,
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

func (stubRunState) Admit(context.Context, execution.RunDraft) error { return nil }
func (stubRunState) Resume(
	context.Context,
	string,
	execution.RunResumeDraft,
	time.Time,
) error {
	return nil
}
func (stubRunState) Suspend(context.Context, transcript.Run) error     { return nil }
func (stubRunState) Terminalize(context.Context, transcript.Run) error { return nil }

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate; these tests have no live turn state to forget.
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
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	// Interrupts is always wired in production (runtime composition root), and
	// the wire status now reads it (liveStatus), so give the stub a real store.
	runtime := &stubRuntime{sess: svc, model: "default-model", interrupts: sqlite.NewInterruptStore(db)}
	return newTestServer(runtime), svc, runtime
}
