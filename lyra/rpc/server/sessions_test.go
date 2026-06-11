package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// stubRuntime satisfies RuntimeServices by embedding it (unstubbed methods
// panic if ever called) and overriding only what the session handlers touch.
type stubRuntime struct {
	RuntimeServices
	sess     session.Service
	model    string
	skills   []engine.SkillInfo
	mcpTools []engine.McpToolInfo
	history  map[string][]chat.Message // per-session chat history (fork copies it)
}

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
func (s stubRuntime) ListSkills(context.Context, string) ([]engine.SkillInfo, error) {
	return s.skills, nil
}

// MCPTools echoes the canned set, applying the same server filter the real
// engine does, so the handler test exercises the scoping passthrough.
func (s stubRuntime) MCPTools(_ context.Context, server string) ([]engine.McpToolInfo, error) {
	if server == "" {
		return s.mcpTools, nil
	}
	var out []engine.McpToolInfo
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
	svc := sqlite.NewSessionService(db)
	return &Server{rt: stubRuntime{sess: svc, model: "default-model"}}, svc
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

// TestForkSession: a full-history fork inherits the parent's cwd, copies its
// history into the child, and honours a title override; an item-boundary fork
// (fromItemId) is rejected as checkpoint_unavailable.
func TestForkSession(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionService(db)
	ctx := context.Background()
	parent, _ := svc.Create(ctx, "research", "/work/proj")

	hist := map[string][]chat.Message{parent.ID: {chat.NewUserMessage("hello"), chat.NewAssistantMessage("hi")}}
	s := &Server{rt: stubRuntime{sess: svc, history: hist}}

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

	// run-boundary fork isn't backed until B4 → checkpoint_unavailable
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: "run_x"}); !errors.Is(err, protocol.ErrCheckpointUnavailable) {
		t.Errorf("fromRunId fork err = %v, want ErrCheckpointUnavailable", err)
	}

	// unknown parent → session_not_found
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: "nope"}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Errorf("unknown parent err = %v, want ErrSessionNotFound", err)
	}
}
