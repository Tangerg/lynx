package server

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
	"github.com/Tangerg/lynx/core/model/chat"
)

// stubRuntime satisfies RuntimeServices by embedding it (unstubbed methods
// panic if ever called) and overriding only what the session handlers touch.
type stubRuntime struct {
	RuntimeServices
	sess        session.Service
	model       string
	skills      []kernel.SkillInfo
	recipes     []recipes.Recipe
	mcpTools    []kernel.McpToolInfo
	mcpStatuses []kernel.McpServerStatus
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        transcript.Store          // durable Item/run history (rollback/fork read runs)
	interrupts  interrupts.Store          // open-interrupt registry (rollback clears dropped)
	chat        turn.Service
}

func (s stubRuntime) MCPServerStatuses() []kernel.McpServerStatus { return s.mcpStatuses }

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

// MessageCount / TruncateMessages operate on the in-memory history map, mirroring
// the engine's chat-memory store closely enough for rollback/fork tests.
func (s stubRuntime) MessageCount(_ context.Context, id string) (int, error) {
	return len(s.history[id]), nil
}

// RunInTx in the stub just runs fn — the in-memory stub has no real
// transaction; production wires the sqlite-backed transactor.
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

// ReconnectMCPServer is a no-op for the stub — WorkspaceMCPReconnect's event
// sequencing is what the server test exercises, and it builds those frames from
// MCPServerStatuses (above), not from this call's side effects.
func (s stubRuntime) ReconnectMCPServer(context.Context, string) error { return nil }

// chatStub satisfies turn.Service by embedding it — most session tests never
// drive a turn, so no method is implemented unless a specific case needs it.
type chatStub struct{ turn.Service }

func (chatStub) Cancel(context.Context, turn.TurnHandle) error { return nil }

type recordingTurns struct {
	turn.Service
	canceled []turn.TurnHandle
}

func (r *recordingTurns) Cancel(_ context.Context, h turn.TurnHandle) error {
	r.canceled = append(r.canceled, h)
	return nil
}

func (s stubRuntime) turnService() turn.Service {
	if s.chat != nil {
		return s.chat
	}
	return chatStub{}
}

func (s stubRuntime) StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error) {
	return s.turnService().StartTurn(ctx, req)
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
	return s.turnService().Events(ctx, handle)
}

func (s stubRuntime) InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error {
	return s.turnService().InjectSteering(ctx, handle, message)
}

func (s stubRuntime) ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return s.turnService().Resume(ctx, handle, resolution)
}

func (s stubRuntime) RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return s.turnService().Rehydrate(ctx, req)
}

func (s stubRuntime) CancelTurn(ctx context.Context, handle turn.TurnHandle) error {
	return s.turnService().Cancel(ctx, handle)
}

func (s stubRuntime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return s.turnService().ProcessID(ctx, handle)
}

func (s stubRuntime) SetTurnInterruptKinds(kinds []string) {
	s.turnService().SetInterruptKinds(kinds)
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

func (s stubRuntime) ClaimRunSlot(ctx context.Context, claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return lifecycle.New(s).ClaimRunSlot(ctx, claims, sessionID)
}

func (s stubRuntime) ClaimMutationSlot(claims lifecycle.SessionClaimer, sessionID string) (lifecycle.RunAdmission, error) {
	return lifecycle.New(s).ClaimMutationSlot(claims, sessionID)
}

func (s stubRuntime) ClaimResumeSlot(ctx context.Context, claims lifecycle.SessionClaimer, parentRunID string) (interrupts.Pending, lifecycle.RunAdmission, error) {
	return lifecycle.New(s).ClaimResumeSlot(ctx, claims, parentRunID)
}

func (s stubRuntime) CancelParkedRun(ctx context.Context, runID string) error {
	return lifecycle.New(s).CancelParkedRun(ctx, stubLifecycleTurns{rt: s}, runID)
}

func (s stubRuntime) CancelRunTurn(ctx context.Context, run lifecycle.RunTurn) error {
	return lifecycle.New(s).CancelRunTurn(ctx, stubLifecycleTurns{rt: s}, run)
}

func (s stubRuntime) ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution) (lifecycle.ResumedInterrupt, error) {
	return lifecycle.New(s).ResumeClaimedInterrupt(ctx, stubLifecycleTurns{rt: s}, parentRunID, resolution)
}

func (s stubRuntime) RollbackResolved(ctx context.Context, sessionID string, boundary lifecycle.RollbackBoundary) error {
	return lifecycle.New(s).RollbackResolved(ctx, stubLifecycleTurns{rt: s}, sessionID, boundary)
}

func (s stubRuntime) ForkSession(ctx context.Context, spec lifecycle.ForkSpec) (session.Session, error) {
	return lifecycle.New(s).Fork(ctx, spec)
}

func (s stubRuntime) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return lifecycle.New(s).RestoreSession(ctx, ses, msgs, runs, items)
}

func (s stubRuntime) DeleteSession(ctx context.Context, id string) error {
	return lifecycle.New(s).DeleteSession(ctx, stubLifecycleTurns{rt: s}, id)
}

func (s stubRuntime) RunSegmentEffects(checkpoints runsegment.Checkpoints, publish runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:             s,
		Processes:          stubRunSegmentProcesses{rt: s},
		Checkpoints:        checkpoints,
		PublishFileChanges: publish,
	})
}

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate — these tests have no live turn state to forget.
func (stubRuntime) ForgetSession(string) {}

func (s stubRuntime) Session() session.Service { return s.sess }
func (s stubRuntime) ListSessions(ctx context.Context) ([]session.Session, error) {
	return s.sess.List(ctx)
}
func (s stubRuntime) GetSession(ctx context.Context, id string) (session.Session, error) {
	return s.sess.Get(ctx, id)
}
func (s stubRuntime) CreateSession(ctx context.Context, title, cwd string) (session.Session, error) {
	return s.sess.Create(ctx, title, cwd)
}
func (s stubRuntime) RenameSession(ctx context.Context, id, title string) error {
	return s.sess.Rename(ctx, id, title)
}
func (s stubRuntime) SetSessionModel(ctx context.Context, id, model string) error {
	return s.sess.SetModel(ctx, id, model)
}
func (s stubRuntime) SetSessionCwd(ctx context.Context, id, cwd string) error {
	return s.sess.SetCwd(ctx, id, cwd)
}
func (s stubRuntime) SetSessionMetadata(ctx context.Context, id string, meta map[string]any) error {
	return s.sess.SetMetadata(ctx, id, meta)
}
func (s stubRuntime) SetSessionFavorite(ctx context.Context, id string, favorite bool) error {
	return s.sess.SetFavorite(ctx, id, favorite)
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
func (s stubRuntime) MCPTools(_ context.Context, server string) ([]kernel.McpToolInfo, error) {
	if server == "" {
		return s.mcpTools, nil
	}
	var out []kernel.McpToolInfo
	for _, t := range s.mcpTools {
		if t.Server == server {
			out = append(out, t)
		}
	}
	return out, nil
}

func newSessionServer(t *testing.T) (*Server, session.Service) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	// Interrupts is always wired in production (runtime composition root) and
	// the wire status now reads it (liveStatus) — give the stub a real store.
	return &Server{rt: stubRuntime{sess: svc, model: "default-model", interrupts: sqlite.NewInterruptStore(db)}}, svc
}

func TestUpdateSession(t *testing.T) {
	s, svc := newSessionServer(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "old", "/w")

	title := "new title"
	out, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Title: &title})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if out.Title != "new title" {
		t.Errorf("Title = %q, want %q", out.Title, "new title")
	}

	// model edit routes to SetModel and surfaces on the wire
	model := "claude-opus-4-8"
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Model: &model})
	if err != nil {
		t.Fatalf("set model: %v", err)
	}
	if out.Model != model {
		t.Errorf("Model = %q, want %q", out.Model, model)
	}

	// whitespace-only title → invalid_params (a session title must be non-empty)
	blank := "   "
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Title: &blank}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("blank title err = %v, want ErrInvalidParams", err)
	}

	// unknown id → session_not_found
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: "nope", Title: &title}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Errorf("unknown id err = %v, want ErrSessionNotFound", err)
	}

	// relocate to a non-existent dir → cwd_unavailable (a stale path would
	// silently break later runs)
	ghost := "/no/such/dir"
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &ghost}); !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Errorf("relocate to ghost err = %v, want ErrCwdUnavailable", err)
	}

	// relocate to a real dir → cwd surfaces on the wire
	newCwd := t.TempDir()
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &newCwd})
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if out.Cwd != newCwd {
		t.Errorf("Cwd = %q, want relocated %q", out.Cwd, newCwd)
	}

	// metadata is full-replaced and round-trips arbitrary JSON values
	meta := map[string]any{"pinned": true, "n": float64(3)}
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Metadata: &meta})
	if err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if out.Metadata["pinned"] != true || out.Metadata["n"] != float64(3) {
		t.Errorf("Metadata = %+v, want {pinned:true, n:3}", out.Metadata)
	}
}

// TestDeleteSession_Cascade verifies a deleted session takes its session-scoped
// data with it: transcript runs+items, chat-memory messages, and open
// interrupts. Without the cascade the sessions row vanishes but those rows
// orphan (the bug: items.list / runs.listOpenInterrupts kept resolving a
// deleted session).
func TestDeleteSession_Cascade(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	svc := sqlite.NewSessionStore(db)
	hist := sqlite.NewTranscriptStore(db)
	ints := sqlite.NewInterruptStore(db)
	created, _ := svc.Create(ctx, "doomed", "/w")
	id := created.ID

	// Seed one of every session-scoped row.
	if err := hist.PutRun(ctx, transcript.Run{SessionID: id, RunID: "run_1", Blob: []byte(`{"id":"run_1"}`)}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := hist.AppendItem(ctx, transcript.Item{SessionID: id, RunID: "run_1", ItemID: "item_1", Blob: []byte(`{"id":"item_1"}`)}); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{ParentRunID: "run_1", SessionID: id, Interrupts: []byte(`[]`)}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	history := map[string][]chat.Message{id: {chat.NewUserMessage("hi")}}

	s := &Server{rt: stubRuntime{sess: svc, hist: hist, interrupts: ints, history: history}}
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := svc.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("session still present after delete: err = %v", err)
	}
	if items, runs, _ := hist.List(ctx, id); len(items) != 0 || len(runs) != 0 {
		t.Errorf("transcript not cascaded: %d items, %d runs left", len(items), len(runs))
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Errorf("interrupts not cascaded: %d left", len(pending))
	}
	if _, ok := history[id]; ok {
		t.Errorf("chat-memory messages not cascaded: still present")
	}
	if s.hasActiveRun(id) {
		t.Fatal("delete leaked the session mutation claim")
	}
}

func TestDeleteSession_RejectsActiveSession(t *testing.T) {
	s, svc := newSessionServer(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, "live", "/w")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.claimSession(created.ID) {
		t.Fatal("claim session")
	}
	t.Cleanup(func() { s.releaseSession(created.ID) })

	if err := s.DeleteSession(ctx, created.ID); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("delete under active claim = %v, want ErrSessionBusy", err)
	}
	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("session mutated under active claim: %v", err)
	}
}

func TestDeleteSession_CancelsParkedTurn(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	svc := sqlite.NewSessionStore(db)
	hist := sqlite.NewTranscriptStore(db)
	ints := sqlite.NewInterruptStore(db)
	created, _ := svc.Create(ctx, "parked", "/w")
	id := created.ID
	if err := ints.Put(ctx, interrupts.Pending{
		ParentRunID: "run_parked",
		SessionID:   id,
		TurnID:      "turn_parked",
		Interrupts:  []byte(`[]`),
	}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	turns := &recordingTurns{}
	s := &Server{rt: stubRuntime{sess: svc, hist: hist, interrupts: ints, history: map[string][]chat.Message{}, chat: turns}}
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(turns.canceled) != 1 {
		t.Fatalf("canceled = %+v, want one parked turn", turns.canceled)
	}
	if got := turns.canceled[0]; got.SessionID != id || got.TurnID != "turn_parked" {
		t.Fatalf("canceled handle = %+v, want %s/turn_parked", got, id)
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Fatalf("pending interrupts = %+v, want cleared", pending)
	}
}

// TestForkSession: a full-history fork inherits the parent's cwd, copies its
// history into the child, and honors a title override; a run-boundary fork
// (fromRunId) against an unknown run is run_not_found.
func TestForkSession(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	ctx := context.Background()
	parent, _ := svc.Create(ctx, "research", "/work/proj")

	hist := map[string][]chat.Message{parent.ID: {chat.NewUserMessage("hello"), chat.NewAssistantMessage("hi")}}
	s := &Server{rt: stubRuntime{sess: svc, history: hist, hist: sqlite.NewTranscriptStore(db)}}

	child, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, Title: "branch A"})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if child.Cwd != "/work/proj" {
		t.Errorf("child cwd = %q, want inherited /work/proj", child.Cwd)
	}
	if child.Title != "branch A" {
		t.Errorf("child title = %q, want override 'branch A'", child.Title)
	}
	if got := len(hist[child.ID]); got != 2 {
		t.Errorf("child history = %d msgs, want 2 copied from parent", got)
	}

	// run-boundary fork against a run that doesn't exist → run_not_found
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: "run_x"}); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Errorf("fromRunId fork err = %v, want ErrRunNotFound", err)
	}

	// unknown parent → session_not_found
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: "nope"}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Errorf("unknown parent err = %v, want ErrSessionNotFound", err)
	}
}
