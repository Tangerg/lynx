package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// sessionRuntimeStore is the runtime package's functional session.Store fake:
// Create/Get return concrete sessions and every mutator records its arguments.
// It backs the turn-planning (PlanTurnStart) and approval-rule tests that still
// read r.sessions off the facade.
type sessionRuntimeStore struct {
	sessions      []session.Session
	getID         string
	createTitle   string
	createCwd     string
	renamed       [2]string
	model         [2]string
	modelErr      error
	cwd           [2]string
	metadataID    string
	metadata      map[string]any
	favoriteID    string
	favoriteValue bool
}

func (s *sessionRuntimeStore) List(context.Context) ([]session.Session, error) {
	return s.sessions, nil
}

func (s *sessionRuntimeStore) Get(_ context.Context, id string) (session.Session, error) {
	s.getID = id
	return session.Session{ID: id, Cwd: "/repo"}, nil
}

func (s *sessionRuntimeStore) Create(_ context.Context, title, cwd string) (session.Session, error) {
	s.createTitle = title
	s.createCwd = cwd
	return session.Session{ID: "ses_created", Title: title, Cwd: cwd}, nil
}

func (*sessionRuntimeStore) Restore(context.Context, session.Session) error { return nil }

func (*sessionRuntimeStore) Fork(context.Context, string, string) (session.Session, error) {
	return session.Session{}, nil
}

func (*sessionRuntimeStore) CreateSubtask(context.Context, string, string) (session.Session, error) {
	return session.Session{}, nil
}

func (*sessionRuntimeStore) Children(context.Context, string) ([]session.Session, error) {
	return nil, nil
}

func (*sessionRuntimeStore) Delete(context.Context, string) error { return nil }

func (s *sessionRuntimeStore) Rename(_ context.Context, id, title string) error {
	s.renamed = [2]string{id, title}
	return nil
}

func (*sessionRuntimeStore) RenameIfUntitled(context.Context, string, string) error { return nil }

func (s *sessionRuntimeStore) SetModel(_ context.Context, id, model string) error {
	s.model = [2]string{id, model}
	return s.modelErr
}

func (s *sessionRuntimeStore) SetCwd(_ context.Context, id, cwd string) error {
	s.cwd = [2]string{id, cwd}
	return nil
}

func (s *sessionRuntimeStore) SetMetadata(_ context.Context, id string, meta map[string]any) error {
	s.metadataID = id
	s.metadata = meta
	return nil
}

func (s *sessionRuntimeStore) SetFavorite(_ context.Context, id string, favorite bool) error {
	s.favoriteID = id
	s.favoriteValue = favorite
	return nil
}

func runtimeWithSessionStore(store *sessionRuntimeStore) *Runtime {
	return &Runtime{sessions: store}
}
