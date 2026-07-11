package sessions

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
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
	metadataID    string
	metadata      map[string]any
	favoriteID    string
	favoriteValue bool
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

func (*crudSessionStore) Restore(context.Context, session.Session) error { return nil }
func (*crudSessionStore) Fork(context.Context, string, string) (session.Session, error) {
	return session.Session{}, nil
}
func (*crudSessionStore) Children(context.Context, string) ([]session.Session, error) {
	return nil, nil
}
func (*crudSessionStore) Delete(context.Context, string) error { return nil }

func (s *crudSessionStore) Rename(_ context.Context, id, title string) error {
	s.renamed = [2]string{id, title}
	return nil
}

func (s *crudSessionStore) SetModel(_ context.Context, id, model string) error {
	s.model = [2]string{id, model}
	return s.modelErr
}

func (s *crudSessionStore) SetCwd(_ context.Context, id, cwd string) error {
	s.cwd = [2]string{id, cwd}
	return nil
}

func (s *crudSessionStore) SetMetadata(_ context.Context, id string, meta map[string]any) error {
	s.metadataID = id
	s.metadata = meta
	return nil
}

func (s *crudSessionStore) SetFavorite(_ context.Context, id string, favorite bool) error {
	s.favoriteID = id
	s.favoriteValue = favorite
	return nil
}

type crudStores struct {
	session *crudSessionStore
	ranInTx bool
}

func (s *crudStores) Session() SessionStore                                     { return s.session }
func (*crudStores) Interrupts() InterruptStore                                  { panic("unused") }
func (*crudStores) ReadHistory(context.Context, string) ([]chat.Message, error) { panic("unused") }
func (*crudStores) SeedHistory(context.Context, string, []chat.Message) error   { panic("unused") }
func (*crudStores) ForgetSession(string)                                        {}
func (s *crudStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	s.ranInTx = true
	return fn(ctx)
}
func (*crudStores) ApplyRollback(context.Context, execution.RollbackPlan) error { panic("unused") }
func (*crudStores) ApplyRestore(context.Context, execution.RestorePlan) error   { panic("unused") }
func (*crudStores) ApplyDelete(context.Context, string) error                   { panic("unused") }
func (*crudStores) ApplyCancel(context.Context, string, string) error           { panic("unused") }

func newCRUDCoordinator(store *crudSessionStore) (*Coordinator, *crudStores) {
	stores := &crudStores{session: store}
	return New(Dependencies{Stores: stores}), stores
}

func TestCoordinatorSessionCRUD(t *testing.T) {
	store := &crudSessionStore{sessions: []session.Session{{ID: "ses_1"}}}
	c, _ := newCRUDCoordinator(store)
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

	createCwd := t.TempDir()
	created, err := c.Create(ctx, "New", filepath.Join(createCwd, "."))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "ses_created" || store.createTitle != "New" || store.createCwd != worktree.CanonicalCwd(createCwd) {
		t.Fatalf("created=%+v title=%q cwd=%q", created, store.createTitle, store.createCwd)
	}
}

func TestCoordinatorUpdateAppliesPatch(t *testing.T) {
	store := &crudSessionStore{}
	c, stores := newCRUDCoordinator(store)
	ctx := context.Background()

	title := "  Renamed  "
	model := "claude-opus-4-8"
	cwd := t.TempDir()
	meta := map[string]any{"pinned": true}
	favorite := true

	got, err := c.Update(ctx, "ses_1", session.Patch{
		Title:    &title,
		Model:    &model,
		Cwd:      &cwd,
		Metadata: &meta,
		Favorite: &favorite,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !stores.ranInTx {
		t.Fatal("Update did not run through the transactor")
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

func TestCoordinatorUpdateRejectsInvalidPatch(t *testing.T) {
	store := &crudSessionStore{}
	c, _ := newCRUDCoordinator(store)

	blank := "  "
	if _, err := c.Update(context.Background(), "ses_1", session.Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("blank title renamed session: %v", store.renamed)
	}

	ghost := "/no/such/dir"
	if _, err := c.Update(context.Background(), "ses_1", session.Patch{Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.cwd != ([2]string{}) {
		t.Fatalf("ghost cwd updated session: %v", store.cwd)
	}

	title := "Renamed"
	if _, err := c.Update(context.Background(), "ses_1", session.Patch{Title: &title, Cwd: &ghost}); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("mixed patch err = %v, want ErrCwdUnavailable", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("invalid mixed patch renamed session: %v", store.renamed)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := c.Create(context.Background(), "New", missing); !errors.Is(err, session.ErrCwdUnavailable) {
		t.Fatalf("missing create cwd err = %v, want ErrCwdUnavailable", err)
	}
	if store.createCwd != "" {
		t.Fatalf("missing create cwd wrote session: %q", store.createCwd)
	}
}
