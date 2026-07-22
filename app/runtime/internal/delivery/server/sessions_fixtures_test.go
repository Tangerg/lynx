package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
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

// stubRuntime is the delivery session/lifecycle test double: it provides the run
// executor + effects (testRuntime) over its own in-memory + sqlite stores, and
// the coordinator-provider seams (sessions / queries / turn control).
type stubRuntime struct {
	sess        *sqlite.SessionStore
	model       string
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        *sqlite.TranscriptStore   // durable Item/run history (rollback/fork read runs)
	toolResults *sqlite.ToolResultStore
	interrupts  *sqlite.InterruptStore         // open-interrupt registry (rollback clears dropped)
	muts        *sqlite.WorkspaceMutationStore // §8.5 recoverable file-rollback log
	turns       turn.Dispatcher
}

// sessionsCoordinatorProvider is the optional test seam newTestServer uses to
// wire s.sessions: a fake that can build the real lifecycle coordinator over its
// own in-memory stores (stubRuntime). Fakes that never drive a lifecycle
// write-set may omit it, leaving s.sessions nil.
type sessionsCoordinatorProvider interface {
	sessionsCoordinator() *sessions.Coordinator
}

// queriesCoordinatorProvider is the parallel seam for the read coordinator: a
// fake that can build the query coordinator over its own transcript/history/
// interrupt stores. Fakes that never drive a read (live-run tests) may omit it.
type queriesCoordinatorProvider interface {
	queriesCoordinator() *queries.Coordinator
}

// stubHistoryReader adapts the stub's in-memory chat-history map to the query
// coordinator's history reader port.
type stubHistoryReader struct{ history map[string][]chat.Message }

func (r stubHistoryReader) Read(_ context.Context, id string) ([]chat.Message, error) {
	return r.history[id], nil
}

func (s stubRuntime) queriesCoordinator() *queries.Coordinator {
	return queries.New(queries.Dependencies{
		Transcript: s.hist,
		History:    stubHistoryReader{history: s.history},
		Interrupts: s.interrupts,
	})
}

func newTestServer(rt testRuntime) *Server {
	s := &Server{}
	// Wire the session/run lifecycle coordinator over the fake's in-memory stores
	// when the fake provides one, mirroring the composition root.
	if p, ok := rt.(sessionsCoordinatorProvider); ok {
		s.sessions = p.sessionsCoordinator()
	}
	var ids atomic.Uint64
	s.coordinator = runs.NewCoordinator(runs.Dependencies{
		Segments: rt,
		Turns:    rt,
		Sessions: s.sessions,
		Effects:  rt.RunSegmentEffects(nil, nil),
		Now:      time.Now,
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
	// Seed a default models coordinator so the session→wire projection (which
	// reads DefaultModel) works; capability handler tests build their own via
	// serverWithModels / serverWithTools / serverWithMCP.
	defaultModel := ""
	if src, ok := rt.(interface{ DefaultModel() string }); ok {
		defaultModel = src.DefaultModel()
	}
	s.models = models.New(models.Config{DefaultModel: defaultModel})
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
func serverWithTools(registry tool.Registry) *Server {
	return &Server{tools: tools.New(registry)}
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

// turnStub satisfies turn.Dispatcher by embedding it. Most session tests never
// drive a turn, so no method is implemented unless a specific case needs it.
type turnStub struct{ turn.Dispatcher }

func (turnStub) Cancel(context.Context, turn.TurnHandle) error { return nil }

type recordingTurns struct {
	turn.Dispatcher
	canceled []turn.TurnHandle
}

func (r *recordingTurns) Cancel(_ context.Context, h turn.TurnHandle) error {
	r.canceled = append(r.canceled, h)
	return nil
}

func (s stubRuntime) turnDispatcher() turn.Dispatcher {
	if s.turns != nil {
		return s.turns
	}
	return turnStub{}
}

func (s stubRuntime) TurnEvents(ctx context.Context, ref runs.TurnRef) (iter.Seq[runs.EngineEvent], error) {
	return turn.NewExecutor(s.turnDispatcher()).TurnEvents(ctx, ref)
}

func (s stubRuntime) ValidateStart(req runs.StartTurn) error {
	return turn.NewExecutor(s.turnDispatcher()).ValidateStart(req)
}

func (s stubRuntime) PrepareStart(ctx context.Context, req runs.StartTurn) (runs.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).PrepareStart(ctx, req)
}

func (s stubRuntime) Activate(ctx context.Context, ref runs.TurnRef) error {
	return turn.NewExecutor(s.turnDispatcher()).Activate(ctx, ref)
}

func (s stubRuntime) Prepare(ctx context.Context, ref runs.TurnRef) (runs.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).Prepare(ctx, ref)
}

func (s stubRuntime) Resume(ctx context.Context, prepared runs.TurnRef, resolution interrupts.Resolution, interruptKinds []string) error {
	return turn.NewExecutor(s.turnDispatcher()).Resume(ctx, prepared, resolution, interruptKinds)
}

func (s stubRuntime) Rehydrate(ctx context.Context, req runs.RehydrateTurn) (runs.TurnRef, error) {
	return turn.NewExecutor(s.turnDispatcher()).Rehydrate(ctx, req)
}

func (s stubRuntime) Cancel(ctx context.Context, ref runs.TurnRef) error {
	return turn.NewExecutor(s.turnDispatcher()).CancelTurn(ctx, ref)
}

func (s stubRuntime) Steer(ctx context.Context, ref runs.TurnRef, message string) error {
	return turn.NewExecutor(s.turnDispatcher()).Steer(ctx, ref, message)
}

func (s stubRuntime) CancelTurn(ctx context.Context, ref runs.TurnRef) error {
	return s.turnDispatcher().Cancel(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

func (s stubRuntime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return s.turnDispatcher().ProcessID(ctx, handle)
}

type stubLifecycleTurns struct {
	rt *stubRuntime
}

func (t stubLifecycleTurns) Cancel(ctx context.Context, ref sessions.RunRef) error {
	return t.rt.CancelTurn(ctx, runs.TurnRef{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

type stubRunSegmentProcesses struct {
	rt stubRuntime
}

func (p stubRunSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.rt.TurnProcessID(ctx, handle)
}

type stubLifecycleStores struct {
	rt *stubRuntime
}

func (s stubLifecycleStores) Session() sessions.SessionStore { return s.rt.sess }

func (s stubLifecycleStores) Interrupts() sessions.InterruptStore { return s.rt.interrupts }

func (s stubLifecycleStores) Transcript() sessions.TranscriptStore { return s.rt.hist }

func (s stubLifecycleStores) ReadSnapshot(ctx context.Context, id string) (sessions.Snapshot, error) {
	ses, err := s.rt.sess.Get(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	items, runs, err := s.rt.hist.List(ctx, id)
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
	return sessions.Snapshot{Session: ses, Messages: messages, Items: items, Runs: runs, ToolResults: toolResults}, nil
}

func (s stubLifecycleStores) ForgetSession(id string) { s.rt.ForgetSession(id) }

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

// The atomic write-sets over the stub's in-memory history + real sqlite
// transcript/interrupt/session stores. The stub carries no durable run-state
// store (admission state is verified in the sqlite/sessions unit tests), so the
// run-state transition is skipped — the observable transcript/history effects
// these delivery tests assert are unaffected.
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
		if err := s.rt.interrupts.Delete(ctx, runID); err != nil {
			return err
		}
	}
	for _, sessionID := range plan.DropSessionIDs {
		if err := s.deleteSession(ctx, sessionID); err != nil {
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
		if err := s.rt.hist.PutRun(ctx, r); err != nil {
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
	return nil
}

func (s stubLifecycleStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	for _, sessionID := range plan.SessionIDs {
		if err := s.deleteSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) deleteSession(ctx context.Context, sessionID string) error {
	if err := s.rt.hist.DeleteSession(ctx, sessionID); err != nil {
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
	for _, item := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, item); err != nil {
			return err
		}
	}
	if err := s.rt.hist.PutRun(ctx, plan.Run); err != nil {
		return err
	}
	return s.rt.interrupts.Delete(ctx, plan.Run.ID)
}

func (s stubLifecycleStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.rt.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.rt.interrupts.Delete(ctx, p.RunID); err != nil {
			return err
		}
	}
	return nil
}

type stubRunSegmentStores struct {
	rt stubRuntime
}

func (s stubRunSegmentStores) Interrupts() runsegment.InterruptStore { return s.rt.interrupts }

func (s stubRunSegmentStores) Session() runsegment.SessionStore { return s.rt.sess }

func (s stubRunSegmentStores) Transcript() runsegment.TranscriptStore { return s.rt.hist }

func (s stubRunSegmentStores) ToolResults() runsegment.ToolResultStore { return s.rt.toolResults }

func (s stubRunSegmentStores) MessageCount(ctx context.Context, id string) (int, error) {
	return s.rt.MessageCount(ctx, id)
}

func (s stubRunSegmentStores) GenerateTitle(context.Context, string) (string, error) {
	return "", nil
}

// sessionsCoordinator builds the real lifecycle coordinator over the stub's
// in-memory stores and turns, so newTestServer can wire s.sessions the way the
// composition root does — delivery drives every lifecycle write-set through it.
// File restore stays disabled (nil restorer); the checkpoint tests rebuild it
// with a real restorer via [stubRuntime.sessionsCoordinatorWithRestorer].
func (s *stubRuntime) sessionsCoordinator() *sessions.Coordinator {
	return s.sessionsCoordinatorWithRestorer(nil)
}

func (s *stubRuntime) sessionsCoordinatorWithRestorer(checkpoints sessions.WorkspaceCheckpoints) *sessions.Coordinator {
	return sessions.New(sessions.Dependencies{
		Stores:      stubLifecycleStores{rt: s},
		Turns:       stubLifecycleTurns{rt: s},
		Paths:       workspacepath.Resolver{},
		Checkpoints: checkpoints,
		Mutations:   s.muts,
	})
}

func (s stubRuntime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:             stubRunSegmentStores{rt: s},
		Processes:          stubRunSegmentProcesses{rt: s},
		RunState:           stubRunState{},
		Tx:                 s.RunInTx,
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
}

type stubRunState struct{}

func (stubRunState) Admit(context.Context, execution.RunDraft) error     { return nil }
func (stubRunState) Resume(context.Context, execution.ResumeDraft) error { return nil }
func (stubRunState) Suspend(context.Context, string, string) error       { return nil }
func (stubRunState) Terminalize(context.Context, string, string, execution.Outcome) error {
	return nil
}

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate; these tests have no live turn state to forget.
func (stubRuntime) ForgetSession(string) {}

func (s stubRuntime) DefaultModel() string { return s.model }
func (s stubRuntime) ReadHistory(_ context.Context, id string) ([]chat.Message, error) {
	return s.history[id], nil
}
func (s stubRuntime) SeedHistory(_ context.Context, id string, msgs []chat.Message) error {
	if s.history != nil {
		s.history[id] = append(s.history[id], msgs...)
	}
	return nil
}

func newSessionServer(t *testing.T) (*Server, *sqlite.SessionStore) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	// Interrupts is always wired in production (runtime composition root), and
	// the wire status now reads it (liveStatus), so give the stub a real store.
	return newTestServer(&stubRuntime{sess: svc, model: "default-model", interrupts: sqlite.NewInterruptStore(db)}), svc
}
