package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

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

func (s *sessionRuntimeStore) Rename(_ context.Context, id, title string) error {
	s.renamed = [2]string{id, title}
	return nil
}

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
	return &Runtime{
		sessionList:     store,
		sessionRead:     store,
		sessionCreation: store,
		sessionPatch:    store,
		sessionModel:    store,
	}
}

func TestRuntimeSessionFacade(t *testing.T) {
	store := &sessionRuntimeStore{sessions: []session.Session{{ID: "ses_1"}}}
	rt := runtimeWithSessionStore(store)
	ctx := context.Background()

	listed, err := rt.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "ses_1" {
		t.Fatalf("listed = %+v", listed)
	}

	got, err := rt.SessionByID(ctx, "ses_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if store.getID != "ses_2" || got.Cwd != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	createCwd := t.TempDir()
	created, err := rt.CreateSession(ctx, "New", filepath.Join(createCwd, "."))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "ses_created" || store.createTitle != "New" || store.createCwd != worktree.CanonicalCwd(createCwd) {
		t.Fatalf("created=%+v title=%q cwd=%q", created, store.createTitle, store.createCwd)
	}

}

func TestRuntimeUpdateSessionAppliesPatch(t *testing.T) {
	store := &sessionRuntimeStore{}
	ranInTx := false
	rt := runtimeWithSessionStore(store)
	rt.transactor = func(ctx context.Context, fn func(context.Context) error) error {
		ranInTx = true
		return fn(ctx)
	}
	ctx := context.Background()

	title := "  Renamed  "
	model := "claude-opus-4-8"
	cwd := t.TempDir()
	meta := map[string]any{"pinned": true}
	favorite := true

	got, err := rt.UpdateSession(ctx, "ses_1", session.Patch{
		Title:    &title,
		Model:    &model,
		Cwd:      &cwd,
		Metadata: &meta,
		Favorite: &favorite,
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if !ranInTx {
		t.Fatal("UpdateSession did not run through transactor")
	}
	if got.ID != "ses_1" || store.renamed != ([2]string{"ses_1", "Renamed"}) {
		t.Fatalf("updated=%+v renamed=%v", got, store.renamed)
	}
	if store.model != ([2]string{"ses_1", model}) {
		t.Fatalf("model = %v", store.model)
	}
	if store.cwd != ([2]string{"ses_1", worktree.CanonicalCwd(cwd)}) {
		t.Fatalf("cwd = %v", store.cwd)
	}
	if store.metadataID != "ses_1" || store.metadata["pinned"] != true {
		t.Fatalf("metadata id=%q meta=%+v", store.metadataID, store.metadata)
	}
	if store.favoriteID != "ses_1" || !store.favoriteValue {
		t.Fatalf("favorite id=%q value=%v", store.favoriteID, store.favoriteValue)
	}
}

func TestRuntimeUpdateSessionRejectsInvalidPatch(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := runtimeWithSessionStore(store)

	blank := "  "
	if _, err := rt.UpdateSession(context.Background(), "ses_1", session.Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("blank title renamed session: %v", store.renamed)
	}

	ghost := "/no/such/dir"
	if _, err := rt.UpdateSession(context.Background(), "ses_1", session.Patch{Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.cwd != ([2]string{}) {
		t.Fatalf("ghost cwd updated session: %v", store.cwd)
	}

	title := "Renamed"
	if _, err := rt.UpdateSession(context.Background(), "ses_1", session.Patch{Title: &title, Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("mixed patch err = %v, want ErrCwdUnavailable", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("invalid mixed patch renamed session: %v", store.renamed)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := rt.CreateSession(context.Background(), "New", missing); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("missing create cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.createCwd != "" {
		t.Fatalf("missing create cwd wrote session: %q", store.createCwd)
	}
}

func TestRuntimeWorkingTreeAdmission(t *testing.T) {
	rt := &Runtime{}
	const cwd = "/repo"

	runAdmission, ok := rt.ClaimWorkingTreeRun(cwd)
	if !ok {
		t.Fatal("run admission must claim an idle cwd")
	}
	if _, ok := rt.ClaimWorkingTreeMutation(cwd); ok {
		t.Fatal("mutation admission must wait for run admission")
	}
	runAdmission.Release()

	mutationAdmission, ok := rt.ClaimWorkingTreeMutation(cwd)
	if !ok {
		t.Fatal("mutation admission must claim an idle cwd")
	}
	if _, ok := rt.ClaimWorkingTreeRun(cwd); ok {
		t.Fatal("run admission must wait for mutation admission")
	}
	mutationAdmission.Release()
}

func TestRuntimeWorkingTreeAdmissionCanonicalizesCwd(t *testing.T) {
	rt := &Runtime{}

	mutationAdmission, ok := rt.ClaimWorkingTreeMutation("/repo/./child/..")
	if !ok {
		t.Fatal("mutation admission must claim canonical cwd")
	}
	defer mutationAdmission.Release()

	if _, ok := rt.ClaimWorkingTreeRun("/repo"); ok {
		t.Fatal("run admission must share the canonical cwd namespace")
	}
}
