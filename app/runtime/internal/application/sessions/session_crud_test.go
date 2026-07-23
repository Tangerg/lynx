package sessions

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type crudSessionStore struct {
	sessions      []session.Session
	getID         string
	createTitle   string
	createCwd     string
	renamed       [2]string
	model         [2]string
	modelErr      error
	cwd           [2]string
	favoriteID    string
	favoriteValue bool
	patched       bool
}

func (s *crudSessionStore) List(context.Context) ([]session.Session, error) { return s.sessions, nil }

func (s *crudSessionStore) Get(_ context.Context, id string) (session.Session, error) {
	s.getID = id
	return session.Session{ID: id, Cwd: "/repo"}, nil
}

func (s *crudSessionStore) Create(_ context.Context, title, cwd string) (session.Session, error) {
	s.createTitle = title
	s.createCwd = cwd
	return session.Session{ID: "ses_created", Title: title, Cwd: cwd}, nil
}

func (*crudSessionStore) Children(context.Context, string) ([]session.Session, error) {
	return nil, nil
}

// Patch applies the normalized patch — recording each set field — as the
// aggregate atomic write-set the coordinator's Update drives.
func (s *crudSessionStore) Patch(_ context.Context, id string, patch session.Patch) (session.Session, error) {
	s.patched = true
	if patch.Title != nil {
		s.renamed = [2]string{id, *patch.Title}
	}
	if patch.Model != nil {
		s.model = [2]string{id, *patch.Model}
	}
	if patch.Cwd != nil {
		s.cwd = [2]string{id, *patch.Cwd}
	}
	if patch.Favorite != nil {
		s.favoriteID = id
		s.favoriteValue = *patch.Favorite
	}
	if s.modelErr != nil {
		return session.Session{}, s.modelErr
	}
	return session.Session{ID: id}, nil
}

type crudStores struct {
	session *crudSessionStore
}

func (s *crudStores) Session() SessionStore                                { return s.session }
func (*crudStores) Interrupts() InterruptStore                             { return nil }
func (*crudStores) Transcript() TranscriptStore                            { return emptyTranscript{} }
func (*crudStores) ReadSnapshot(context.Context, string) (Snapshot, error) { return Snapshot{}, nil }
func (*crudStores) ForgetSession(string)                                   {}
func (*crudStores) ApplyFork(context.Context, ForkPlan) (session.Session, error) {
	return session.Session{}, nil
}
func (*crudStores) ApplyRollback(context.Context, RollbackPlan) error { return nil }
func (*crudStores) ApplyRestore(context.Context, RestorePlan) error   { return nil }
func (*crudStores) ApplyDelete(context.Context, DeletePlan) error     { return nil }
func (*crudStores) ApplyTerminal(context.Context, TerminalPlan) error { return nil }

func TestCoordinatorSessionCRUD(t *testing.T) {
	store := &crudSessionStore{sessions: []session.Session{{ID: "ses_1"}}}
	stores := &crudStores{session: store}
	c := New(testDependencies(stores, Dependencies{Paths: testCwdResolver{resolved: "/resolved/project"}}))
	ctx := context.Background()

	listed, err := c.List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "ses_1" {
		t.Fatalf("listed = %+v", listed)
	}

	got, err := c.Get(ctx, "ses_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if store.getID != "ses_2" || got.Cwd != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	created, err := c.Create(ctx, "New", "/requested/project")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "ses_created" || store.createTitle != "New" || store.createCwd != "/resolved/project" {
		t.Fatalf("created=%+v title=%q cwd=%q", created, store.createTitle, store.createCwd)
	}
}

func TestViewUsesConfiguredDefaultModel(t *testing.T) {
	c := New(Dependencies{Paths: testCwdResolver{}, DefaultModel: "claude-opus-4-8"})

	view, err := c.view(t.Context(), session.Session{ID: "ses_1", Cwd: "/repo"}, SessionIdle)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Model != "claude-opus-4-8" {
		t.Fatalf("view model = %q, want configured default", view.Model)
	}
}

func TestCoordinatorUpdateAppliesPatch(t *testing.T) {
	store := &crudSessionStore{}
	claims := new(testClaimer)
	stores := &crudStores{session: store}
	c := New(testDependencies(stores, Dependencies{Paths: testCwdResolver{resolved: "/resolved/project"}, Admissions: claims}))
	ctx := context.Background()

	title := "  Renamed  "
	model := "claude-opus-4-8"
	cwd := "/requested/project"
	favorite := true

	got, err := c.Update(ctx, "ses_1", session.Patch{
		Title:    &title,
		Model:    &model,
		Cwd:      &cwd,
		Favorite: &favorite,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !store.patched {
		t.Fatal("Update did not apply the atomic patch write-set")
	}
	if got.ID != "ses_1" || store.renamed != ([2]string{"ses_1", "Renamed"}) {
		t.Fatalf("updated=%+v renamed=%v", got, store.renamed)
	}
	if store.model != ([2]string{"ses_1", model}) {
		t.Fatalf("model = %v", store.model)
	}
	if store.cwd != ([2]string{"ses_1", "/resolved/project"}) {
		t.Fatalf("cwd = %v", store.cwd)
	}
	if store.favoriteID != "ses_1" || !store.favoriteValue {
		t.Fatalf("favorite id=%q value=%v", store.favoriteID, store.favoriteValue)
	}
	if len(claims.released) != 1 || claims.released[0] != "ses_1" {
		t.Fatalf("relocation admission releases = %v, want [ses_1]", claims.released)
	}
}

func TestCoordinatorUpdateRejectsRelocationDuringRun(t *testing.T) {
	store := &crudSessionStore{}
	claims := &testClaimer{claimed: map[string]bool{"ses_1": true}}
	stores := &crudStores{session: store}
	c := New(testDependencies(stores, Dependencies{Paths: testCwdResolver{resolved: "/resolved/project"}, Admissions: claims}))
	cwd := "/requested/project"

	_, err := c.Update(t.Context(), "ses_1", session.Patch{Cwd: &cwd})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Update relocation error = %v, want ErrSessionBusy", err)
	}
	if store.patched {
		t.Fatal("busy relocation mutated the session")
	}
}

func TestCoordinatorUpdateRejectsInvalidPatch(t *testing.T) {
	store := &crudSessionStore{}
	stores := &crudStores{session: store}
	c := New(testDependencies(stores, Dependencies{Paths: testCwdResolver{err: errors.New("cwd unavailable")}}))

	blank := "  "
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("blank title renamed session: %v", store.renamed)
	}

	ghost := "/no/such/dir"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.cwd != ([2]string{}) {
		t.Fatalf("ghost cwd updated session: %v", store.cwd)
	}

	title := "Renamed"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &title, Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("mixed patch err = %v, want ErrCwdUnavailable", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("invalid mixed patch renamed session: %v", store.renamed)
	}

	missing := "/missing/project"
	if _, err := c.Create(context.Background(), "New", missing); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("missing create cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.createCwd != "" {
		t.Fatalf("missing create cwd wrote session: %q", store.createCwd)
	}
}
