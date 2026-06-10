package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// stubRuntime satisfies RuntimeServices by embedding it (unstubbed methods
// panic if ever called) and overriding only what the session handlers touch.
type stubRuntime struct {
	RuntimeServices
	sess   session.Service
	model  string
	skills []engine.SkillInfo
}

func (s stubRuntime) Session() session.Service { return s.sess }
func (s stubRuntime) DefaultModel() string     { return s.model }
func (s stubRuntime) ListSkills(context.Context, string) ([]engine.SkillInfo, error) {
	return s.skills, nil
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

	// cwd relocate is gated off → capability_not_negotiated
	cwd := "/elsewhere"
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &cwd}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Errorf("relocate err = %v, want ErrCapabilityNotNeg", err)
	}
}
