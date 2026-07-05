package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type sessionRuntimeStore struct {
	session.Service
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

func TestRuntimeSessionFacade(t *testing.T) {
	store := &sessionRuntimeStore{sessions: []session.Session{{ID: "ses_1"}}}
	rt := &Runtime{session: store}
	ctx := context.Background()

	listed, err := rt.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "ses_1" {
		t.Fatalf("listed = %+v", listed)
	}

	got, err := rt.GetSession(ctx, "ses_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if store.getID != "ses_2" || got.Cwd != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	created, err := rt.CreateSession(ctx, "New", "/repo")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "ses_created" || store.createTitle != "New" || store.createCwd != "/repo" {
		t.Fatalf("created=%+v title=%q cwd=%q", created, store.createTitle, store.createCwd)
	}

	if err := rt.RenameSession(ctx, "ses_1", "Renamed"); err != nil {
		t.Fatalf("rename session: %v", err)
	}
	if store.renamed != ([2]string{"ses_1", "Renamed"}) {
		t.Fatalf("renamed = %v", store.renamed)
	}

	if err := rt.SetSessionModel(ctx, "ses_1", "claude-opus-4-8"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if store.model != ([2]string{"ses_1", "claude-opus-4-8"}) {
		t.Fatalf("model = %v", store.model)
	}

	if err := rt.SetSessionCwd(ctx, "ses_1", "/new"); err != nil {
		t.Fatalf("set cwd: %v", err)
	}
	if store.cwd != ([2]string{"ses_1", "/new"}) {
		t.Fatalf("cwd = %v", store.cwd)
	}

	meta := map[string]any{"pinned": true}
	if err := rt.SetSessionMetadata(ctx, "ses_1", meta); err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if store.metadataID != "ses_1" || store.metadata["pinned"] != true {
		t.Fatalf("metadata id=%q meta=%+v", store.metadataID, store.metadata)
	}

	if err := rt.SetSessionFavorite(ctx, "ses_1", true); err != nil {
		t.Fatalf("set favorite: %v", err)
	}
	if store.favoriteID != "ses_1" || !store.favoriteValue {
		t.Fatalf("favorite id=%q value=%v", store.favoriteID, store.favoriteValue)
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
