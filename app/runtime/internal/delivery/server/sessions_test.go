package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
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

// chatStub satisfies turn.Service by embedding it — these tests never drive a
// turn, so no method is implemented.
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

func (s stubRuntime) Chat() turn.Service {
	if s.chat != nil {
		return s.chat
	}
	return chatStub{}
}

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate — these tests have no live turn state to forget.
func (stubRuntime) ForgetSession(string) {}

func (s stubRuntime) Session() session.Service { return s.sess }
func (s stubRuntime) DefaultModel() string     { return s.model }
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
