package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// sessionRuntimeStore is the runtime package's fake for the facade's
// [sessionStore] port: Create/Get return concrete sessions and SetModel records
// its arguments. It backs the turn-planning (PlanTurnStart) and per-run-model
// tests that read r.sessions off the facade.
type sessionRuntimeStore struct {
	getID       string
	createTitle string
	createCwd   string
	model       [2]string
	modelErr    error
}

var _ sessionStore = (*sessionRuntimeStore)(nil)

func (s *sessionRuntimeStore) Get(_ context.Context, id string) (session.Session, error) {
	s.getID = id
	return session.Session{ID: id, Cwd: "/repo"}, nil
}

func (s *sessionRuntimeStore) Create(_ context.Context, title, cwd string) (session.Session, error) {
	s.createTitle = title
	s.createCwd = cwd
	return session.Session{ID: "ses_created", Title: title, Cwd: cwd}, nil
}

func (s *sessionRuntimeStore) SetModel(_ context.Context, id, model string) error {
	s.model = [2]string{id, model}
	return s.modelErr
}

func (*sessionRuntimeStore) RenameIfUntitled(context.Context, string, string) error { return nil }

func runtimeWithSessionStore(store *sessionRuntimeStore) *Runtime {
	return &Runtime{sessions: store}
}
