package server

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
	"github.com/Tangerg/lynx/core/model/chat"
)

// stubRuntime satisfies RuntimePort by embedding it (unstubbed methods panic if
// ever called) and overriding only what the session handlers touch.
type stubRuntime struct {
	RuntimePort
	sess        session.Store
	model       string
	skills      []kernel.SkillInfo
	recipes     []recipes.Recipe
	mcpTools    []kernel.MCPToolInfo
	mcpStatuses []kernel.MCPServerStatus
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        transcript.Store          // durable Item/run history (rollback/fork read runs)
	interrupts  interrupts.Store          // open-interrupt registry (rollback clears dropped)
	turns       turn.Dispatcher
	workingTree *lifecycle.WorkingTreeGate
}

func newTestServer(rt RuntimePort) *Server {
	return &Server{runtimeBindings: bindRuntime(rt)}
}

func newTestServerWithInfo(rt RuntimePort, info protocol.ServerInfo) *Server {
	s := newTestServer(rt)
	s.serverInfo = info
	return s
}

func (s stubRuntime) MCPServerStatuses() []kernel.MCPServerStatus { return s.mcpStatuses }
func (stubRuntime) SupportedProviders() []provider.Metadata       { return nil }
func (stubRuntime) ProviderMetadata(string) (provider.Metadata, bool) {
	return provider.Metadata{}, false
}

func (s stubRuntime) Transcript() transcript.Store { return s.hist }
func (s stubRuntime) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	if s.hist == nil {
		return nil, nil, nil
	}
	return s.hist.List(ctx, sessionID)
}
func (s stubRuntime) ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	if s.hist == nil {
		return nil, nil
	}
	return s.hist.ListRuns(ctx, sessionID)
}
func (s stubRuntime) Interrupts() interrupts.Store { return s.interrupts }
func (s stubRuntime) ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	if s.interrupts == nil {
		return nil, nil
	}
	return s.interrupts.List(ctx, sessionID)
}

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

// ReconnectMCPServer is a no-op for the stub. WorkspaceMCPReconnect's event
// sequencing is what the server test exercises, and it builds those frames from
// MCPServerStatuses (above), not from this call's side effects.
func (s stubRuntime) ReconnectMCPServer(context.Context, string) error { return nil }

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

func (s stubRuntime) StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error) {
	return s.turnDispatcher().StartTurn(ctx, req)
}

func (s stubRuntime) PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (session.Session, turn.StartTurnRequest, error) {
	if draft.Message == "" && len(draft.Media) == 0 {
		return session.Session{}, turn.StartTurnRequest{}, turn.ErrInputRequired
	}
	if (draft.Model == "") != (draft.Provider == "") {
		return session.Session{}, turn.StartTurnRequest{}, turn.ErrIncompleteModelSelection
	}
	var (
		sess session.Session
		err  error
	)
	if sessionID == "" {
		sess, err = s.sess.Create(ctx, "", defaultCwd)
	} else {
		sess, err = s.sess.Get(ctx, sessionID)
	}
	if err != nil {
		return session.Session{}, turn.StartTurnRequest{}, err
	}
	planned := draft
	planned.SessionID = sess.ID
	planned.Cwd = sess.Cwd
	return sess, planned, nil
}

func (s stubRuntime) TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error) {
	return s.turnDispatcher().Events(ctx, handle)
}

func (s stubRuntime) InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error {
	return s.turnDispatcher().InjectSteering(ctx, handle, message)
}

func (s stubRuntime) ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return s.turnDispatcher().Resume(ctx, handle, resolution)
}

func (s stubRuntime) RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return s.turnDispatcher().Rehydrate(ctx, req)
}

func (s stubRuntime) CancelTurn(ctx context.Context, handle turn.TurnHandle) error {
	return s.turnDispatcher().Cancel(ctx, handle)
}

func (s stubRuntime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return s.turnDispatcher().ProcessID(ctx, handle)
}

func (s stubRuntime) SetTurnInterruptKinds(kinds []string) {
	s.turnDispatcher().SetInterruptKinds(kinds)
}

type stubLifecycleTurns struct {
	rt stubRuntime
}

func (t stubLifecycleTurns) Cancel(ctx context.Context, handle turn.TurnHandle) error {
	return t.rt.CancelTurn(ctx, handle)
}

func (t stubLifecycleTurns) Resume(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return t.rt.ResumeTurn(ctx, handle, resolution)
}

func (t stubLifecycleTurns) Rehydrate(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return t.rt.RehydrateTurn(ctx, req)
}

type stubRunSegmentProcesses struct {
	rt stubRuntime
}

func (p stubRunSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.rt.TurnProcessID(ctx, handle)
}

type stubLifecycleStores struct {
	rt stubRuntime
}

func (s stubLifecycleStores) Session() lifecycle.SessionStore { return s.rt.sess }

func (s stubLifecycleStores) Transcript() lifecycle.TranscriptStore { return s.rt.hist }

func (s stubLifecycleStores) Interrupts() lifecycle.InterruptStore { return s.rt.interrupts }

func (s stubLifecycleStores) ReadHistory(ctx context.Context, id string) ([]chat.Message, error) {
	return s.rt.ReadHistory(ctx, id)
}

func (s stubLifecycleStores) TruncateMessages(ctx context.Context, id string, keepN int) error {
	return s.rt.TruncateMessages(ctx, id, keepN)
}

func (s stubLifecycleStores) SeedHistory(ctx context.Context, id string, msgs []chat.Message) error {
	return s.rt.SeedHistory(ctx, id, msgs)
}

func (s stubLifecycleStores) ForgetSession(id string) { s.rt.ForgetSession(id) }

func (s stubLifecycleStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return s.rt.RunInTx(ctx, fn)
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

func (s stubRuntime) ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return lifecycle.New(stubLifecycleStores{rt: s}).ClaimRunSlot(ctx, claims, sessionID)
}

func (s stubRuntime) ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return lifecycle.New(stubLifecycleStores{rt: s}).ClaimMutationSlot(claims, sessionID)
}

func (s *stubRuntime) ClaimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return s.workingTreeGate().ClaimRun(worktree.CanonicalCwd(cwd))
}

func (s *stubRuntime) ClaimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return s.workingTreeGate().ClaimMutation(worktree.CanonicalCwd(cwd))
}

func (s *stubRuntime) workingTreeGate() *lifecycle.WorkingTreeGate {
	if s.workingTree == nil {
		s.workingTree = &lifecycle.WorkingTreeGate{}
	}
	return s.workingTree
}

func (s stubRuntime) ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error) {
	return lifecycle.New(stubLifecycleStores{rt: s}).ClaimResumeSlot(ctx, claims, parentRunID)
}

func (s stubRuntime) CancelParkedRun(ctx context.Context, runID string) error {
	return lifecycle.New(stubLifecycleStores{rt: s}).CancelParkedRun(ctx, stubLifecycleTurns{rt: s}, runID)
}

func (s stubRuntime) CancelRunBinding(ctx context.Context, run lifecycle.RunTurnBinding) error {
	return lifecycle.New(stubLifecycleStores{rt: s}).CancelRunBinding(ctx, stubLifecycleTurns{rt: s}, run)
}

func (s stubRuntime) ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error) {
	return lifecycle.New(stubLifecycleStores{rt: s}).ResumeClaimedInterrupt(ctx, stubLifecycleTurns{rt: s}, parentRunID, resolution)
}

func (s stubRuntime) RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error {
	return lifecycle.New(stubLifecycleStores{rt: s}).RollbackResolved(ctx, stubLifecycleTurns{rt: s}, sessionID, boundary)
}

func (s stubRuntime) ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (session.Session, error) {
	return lifecycle.New(stubLifecycleStores{rt: s}).Fork(ctx, spec)
}

func (s stubRuntime) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return lifecycle.New(stubLifecycleStores{rt: s}).RestoreSession(ctx, ses, msgs, runs, items)
}

func (s stubRuntime) DeleteSession(ctx context.Context, id string) error {
	return lifecycle.New(stubLifecycleStores{rt: s}).DeleteSession(ctx, stubLifecycleTurns{rt: s}, id)
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

func (s stubRuntime) ListSessions(ctx context.Context) ([]session.Session, error) {
	return s.sess.List(ctx)
}
func (s stubRuntime) SessionByID(ctx context.Context, id string) (session.Session, error) {
	return s.sess.Get(ctx, id)
}
func (s stubRuntime) CreateSession(ctx context.Context, title, cwd string) (session.Session, error) {
	return s.sess.Create(ctx, title, cwd)
}
func (s stubRuntime) UpdateSession(ctx context.Context, id string, patch session.Patch) (session.Session, error) {
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return session.Session{}, session.ErrTitleRequired
		}
		if err := s.sess.Rename(ctx, id, title); err != nil {
			return session.Session{}, err
		}
	}
	if patch.Model != nil {
		if err := s.sess.SetModel(ctx, id, *patch.Model); err != nil {
			return session.Session{}, err
		}
	}
	if patch.Cwd != nil {
		cwd, err := worktree.ResolveExistingDir(*patch.Cwd)
		if err != nil {
			return session.Session{}, errors.Join(session.ErrCwdUnavailable, err)
		}
		if err := s.sess.SetCwd(ctx, id, cwd); err != nil {
			return session.Session{}, err
		}
	}
	if patch.Metadata != nil {
		if err := s.sess.SetMetadata(ctx, id, *patch.Metadata); err != nil {
			return session.Session{}, err
		}
	}
	if patch.Favorite != nil {
		if err := s.sess.SetFavorite(ctx, id, *patch.Favorite); err != nil {
			return session.Session{}, err
		}
	}
	return s.sess.Get(ctx, id)
}
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
func (s stubRuntime) ListSkills(context.Context, string) ([]kernel.SkillInfo, error) {
	return s.skills, nil
}
func (s stubRuntime) ListRecipes(context.Context, string) ([]recipes.Recipe, error) {
	return s.recipes, nil
}

// MCPTools echoes the canned set, applying the same server filter the real
// engine does, so the handler test exercises the scoping passthrough.
func (s stubRuntime) MCPTools(_ context.Context, server string) ([]kernel.MCPToolInfo, error) {
	if server == "" {
		return s.mcpTools, nil
	}
	var out []kernel.MCPToolInfo
	for _, t := range s.mcpTools {
		if t.Server == server {
			out = append(out, t)
		}
	}
	return out, nil
}

func newSessionServer(t *testing.T) (*Server, session.Store) {
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
