package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/application/capabilities"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/model/chat"
)

// testRuntime is the delivery test seam newTestServer builds the run coordinator
// from: the executor + the run-segment effects factory. Production wires the
// agentexec turn executor + a Host-built effects; the stub provides both, plus
// the optional coordinator-provider seams asserted below.
type testRuntime interface {
	runs.Executor
	RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects
}

// stubRuntime is the delivery session/lifecycle test double: it provides the run
// executor + effects (testRuntime) over its own in-memory + sqlite stores, and
// the coordinator-provider seams (sessions / queries / turn control).
type stubRuntime struct {
	sess       *sqlite.SessionStore
	model      string
	history    map[string][]chat.Message      // per-session chat history (fork copies it)
	hist       *sqlite.TranscriptStore        // durable Item/run history (rollback/fork read runs)
	interrupts *sqlite.InterruptStore         // open-interrupt registry (rollback clears dropped)
	muts       *sqlite.WorkspaceMutationStore // §8.5 recoverable file-rollback log
	turns      turn.Dispatcher
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

// turnControlProvider is the parallel seam for the turn-start adapter: a fake that
// can build the real turn.Control over its dispatcher + session store.
type turnControlProvider interface {
	turnControlAdapter() *turn.Control
}

func (s stubRuntime) turnControlAdapter() *turn.Control {
	return turn.NewControl(s.turnDispatcher(), s.sess)
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
	// Build the run Coordinator like the Host does, so tests exercise the real
	// admission / lifecycle seam. The stub runtime provides both the executor
	// (TurnEvents/CancelTurn) and the run-segment effects; no checkpoint store or
	// file-change publisher is needed for these tests.
	s.coordinator = runs.NewCoordinator(rt, rt.RunSegmentEffects(nil, nil), nil)
	// Wire the session/run lifecycle coordinator over the fake's in-memory stores
	// when the fake provides one, mirroring the composition root.
	if p, ok := rt.(sessionsCoordinatorProvider); ok {
		s.sessions = p.sessionsCoordinator()
	}
	if p, ok := rt.(queriesCoordinatorProvider); ok {
		s.queries = p.queriesCoordinator()
	}
	if p, ok := rt.(turnControlProvider); ok {
		s.turnControl = p.turnControlAdapter()
	}
	// Seed a default models coordinator so the session→wire projection (which
	// reads DefaultModel) works; capability handler tests build their own via
	// serverWithModels / serverWithCapabilities.
	defaultModel := ""
	if src, ok := rt.(interface{ DefaultModel() string }); ok {
		defaultModel = src.DefaultModel()
	}
	s.models = models.New(models.Config{DefaultModel: defaultModel})
	// Default to a disabled schedules coordinator (schedules.* report
	// capability_not_negotiated); schedule tests replace it with a fake registry.
	s.schedules = schedules.NewCoordinator(nil, nil)
	return s
}

// serverWithCapabilities builds a Server whose only wired coordinator is the
// capabilities one — enough for the tools handler tests, which touch nothing else.
func serverWithCapabilities(cfg capabilities.Config) *Server {
	return &Server{capabilities: capabilities.New(cfg)}
}

// serverWithModels builds a Server whose only wired coordinator is the models one
// — enough for the providers / models handler tests.
func serverWithModels(cfg models.Config) *Server {
	return &Server{models: models.New(cfg)}
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

func (s stubRuntime) TurnEvents(ctx context.Context, handle runs.Handle) (iter.Seq[runs.EngineEvent], error) {
	h, ok := handle.(turn.TurnHandle)
	if !ok {
		return nil, fmt.Errorf("stub: handle %T is not a turn handle", handle)
	}
	seq, err := s.turnDispatcher().Events(ctx, h)
	if err != nil {
		return nil, err
	}
	return func(yield func(runs.EngineEvent) bool) {
		for ev := range seq {
			if !yield(ev) {
				return
			}
		}
	}, nil
}

func (s stubRuntime) ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution, interruptKinds []string) error {
	return s.turnDispatcher().Resume(ctx, handle, resolution, interruptKinds)
}

func (s stubRuntime) RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return s.turnDispatcher().Rehydrate(ctx, req)
}

func (s stubRuntime) CancelTurn(ctx context.Context, handle runs.Handle) error {
	h, ok := handle.(turn.TurnHandle)
	if !ok {
		return fmt.Errorf("stub: handle %T is not a turn handle", handle)
	}
	return s.turnDispatcher().Cancel(ctx, h)
}

func (s stubRuntime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return s.turnDispatcher().ProcessID(ctx, handle)
}

type stubLifecycleTurns struct {
	rt *stubRuntime
}

func (t stubLifecycleTurns) Cancel(ctx context.Context, ref sessions.RunRef) error {
	return t.rt.CancelTurn(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
}

func (t stubLifecycleTurns) Resume(ctx context.Context, ref sessions.RunRef, resolution interrupts.Resolution, interruptKinds []string) (sessions.Handle, error) {
	handle := turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
	return handle, mapStubResumeError(t.rt.ResumeTurn(ctx, handle, resolution, interruptKinds))
}

func (t stubLifecycleTurns) Rehydrate(ctx context.Context, req sessions.RehydrateSpec) (sessions.Handle, error) {
	handle, err := t.rt.RehydrateTurn(ctx, turn.RehydrateRequest{
		SessionID:      req.SessionID,
		ProcessID:      req.ProcessID,
		Approved:       req.Approved,
		Provider:       req.Provider,
		Model:          req.Model,
		InterruptKinds: req.InterruptKinds,
	})
	return handle, mapStubResumeError(err)
}

// mapStubResumeError mirrors the production bootstrap adapter: it maps the turn
// dispatcher's resume vocabulary onto the coordinator's neutral sentinels so the
// delivery resume tests branch exactly as production does.
func mapStubResumeError(err error) error {
	switch {
	case errors.Is(err, turn.ErrParkClaimed):
		return fmt.Errorf("%w: %w", sessions.ErrParkClaimed, err)
	case errors.Is(err, turn.ErrTurnNotFound):
		return fmt.Errorf("%w: %w", sessions.ErrTurnNotLive, err)
	case errors.Is(err, turn.ErrRehydrateCommitted):
		return fmt.Errorf("%w: %w", sessions.ErrRehydrateCommitted, err)
	default:
		return err
	}
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

func (s stubLifecycleStores) ReadHistory(ctx context.Context, id string) ([]chat.Message, error) {
	return s.rt.ReadHistory(ctx, id)
}

func (s stubLifecycleStores) ForgetSession(id string) { s.rt.ForgetSession(id) }

func (s stubLifecycleStores) ApplyFork(ctx context.Context, plan execution.ForkPlan) (session.Session, error) {
	child, err := s.rt.sess.Fork(ctx, plan.ParentID, "")
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
func (s stubLifecycleStores) ApplyRollback(ctx context.Context, plan execution.RollbackPlan) error {
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
	return nil
}

func (s stubLifecycleStores) ApplyRestore(ctx context.Context, plan execution.RestorePlan) error {
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
	return nil
}

func (s stubLifecycleStores) ApplyDelete(ctx context.Context, sessionID string) error {
	if err := s.rt.hist.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rt.TruncateMessages(ctx, sessionID, 0); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, sessionID); err != nil {
		return err
	}
	return s.rt.sess.Delete(ctx, sessionID)
}

func (s stubLifecycleStores) ApplyCancel(ctx context.Context, _ string, runID string) error {
	return s.rt.interrupts.Delete(ctx, runID)
}

func (s stubLifecycleStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.rt.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.rt.interrupts.Delete(ctx, p.ParentRunID); err != nil {
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
		Checkpoints: checkpoints,
		Mutations:   s.muts,
	})
}

func (s stubRuntime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:             stubRunSegmentStores{rt: s},
		Processes:          stubRunSegmentProcesses{rt: s},
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
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
