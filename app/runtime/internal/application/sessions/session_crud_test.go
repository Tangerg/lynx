package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/pagination"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type crudSessionStore struct {
	sessions      []session.Session
	getID         string
	createTitle   string
	createCWD     string
	renamed       [2]string
	model         [2]string
	modelErr      error
	cwd           [2]string
	favoriteID    string
	favoriteValue bool
	isolated      *bool
	patched       bool
}

func (s *crudSessionStore) ListPage(ctx context.Context, _ bool, _ int64, _ string, _ int) ([]session.Session, error) {
	return s.List(ctx)
}

func (s *crudSessionStore) List(context.Context) ([]session.Session, error) { return s.sessions, nil }

func (s *crudSessionStore) Get(_ context.Context, id string) (session.Session, error) {
	s.getID = id
	return session.Session{ID: id, CWD: "/repo"}, nil
}

func (s *crudSessionStore) Create(_ context.Context, title, cwd string) (session.Session, error) {
	s.createTitle = title
	s.createCWD = cwd
	return session.Session{ID: "ses_created", Title: title, CWD: cwd}, nil
}

func (s *crudSessionStore) Ensure(_ context.Context, sess session.Session) (session.Session, error) {
	return sess, nil
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
	if patch.CWD != nil {
		s.cwd = [2]string{id, *patch.CWD}
	}
	if patch.Favorite != nil {
		s.favoriteID = id
		s.favoriteValue = *patch.Favorite
	}
	if patch.Isolated != nil {
		isolated := *patch.Isolated
		s.isolated = &isolated
	}
	if s.modelErr != nil {
		return session.Session{}, s.modelErr
	}
	return session.Session{ID: id}, nil
}

type crudStores struct {
	// session is the port, not the fake: one test pages through a store that seeks,
	// and the harness has no reason to care which fake it is holding.
	session    Store
	interrupts InterruptStore
}

func (s *crudStores) Session() Store { return s.session }
func (s *crudStores) Interrupts() InterruptStore {
	if s.interrupts != nil {
		return s.interrupts
	}
	return &coordinatorInterrupts{pending: map[string]runs.Pending{}}
}
func (*crudStores) Transcript() TranscriptStore                            { return emptyTranscript{} }
func (*crudStores) Runs() RunStore                                         { return emptyTranscript{} }
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
	c := New(testDependencies(stores, Dependencies{Paths: testCWDResolver{resolved: "/resolved/project"}}))
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
	if store.getID != "ses_2" || got.CWD != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	created, err := c.Create(ctx, "New", "/requested/project")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "ses_created" || store.createTitle != "New" || store.createCWD != "/resolved/project" {
		t.Fatalf("created=%+v title=%q cwd=%q", created, store.createTitle, store.createCWD)
	}
}

func TestViewUsesConfiguredDefaultModel(t *testing.T) {
	c := New(Dependencies{Paths: testCWDResolver{}, DefaultModel: "claude-opus-4-8"})

	view, err := c.view(session.Session{ID: "ses_1", CWD: "/repo"}, ActivityIdle)
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
	c := New(testDependencies(stores, Dependencies{Paths: testCWDResolver{resolved: "/resolved/project"}, Admissions: claims}))
	ctx := context.Background()

	title := "  Renamed  "
	model := "claude-opus-4-8"
	cwd := "/requested/project"
	favorite := true

	got, err := c.Update(ctx, "ses_1", session.Patch{
		Title:    &title,
		Model:    &model,
		CWD:      &cwd,
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
	c := New(testDependencies(stores, Dependencies{Paths: testCWDResolver{resolved: "/resolved/project"}, Admissions: claims}))
	cwd := "/requested/project"

	_, err := c.Update(t.Context(), "ses_1", session.Patch{CWD: &cwd})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Update relocation error = %v, want ErrSessionBusy", err)
	}
	if store.patched {
		t.Fatal("busy relocation mutated the session")
	}
}

func TestCoordinatorUpdateRejectsExecutionPolicyChangeWhileParked(t *testing.T) {
	for name, patch := range map[string]session.Patch{
		"cwd": func() session.Patch {
			cwd := "/requested/project"
			return session.Patch{CWD: &cwd}
		}(),
		"isolation": func() session.Patch {
			isolated := true
			return session.Patch{Isolated: &isolated}
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			store := &crudSessionStore{}
			stores := &crudStores{
				session: store,
				interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{
					"run_1": {RootRunID: "run_1", SessionID: "ses_1"},
				}},
			}
			coordinator := New(testDependencies(stores, Dependencies{
				Paths:      testCWDResolver{resolved: "/resolved/project"},
				Admissions: new(testClaimer),
			}))

			_, err := coordinator.Update(t.Context(), "ses_1", patch)
			if !errors.Is(err, ErrSessionBusy) {
				t.Fatalf("Update error = %v, want ErrSessionBusy", err)
			}
			if store.patched {
				t.Fatal("parked execution policy change mutated the Session")
			}
		})
	}
}

func TestCoordinatorUpdateRejectsInvalidPatch(t *testing.T) {
	store := &crudSessionStore{}
	stores := &crudStores{session: store}
	c := New(testDependencies(stores, Dependencies{Paths: testCWDResolver{err: errors.New("cwd unavailable")}}))

	blank := "  "
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("blank title renamed session: %v", store.renamed)
	}

	ghost := "/no/such/dir"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{CWD: &ghost}); !errors.Is(err, session.ErrCWDUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCWDUnavailable", err)
	}
	if store.cwd != ([2]string{}) {
		t.Fatalf("ghost cwd updated session: %v", store.cwd)
	}

	title := "Renamed"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &title, CWD: &ghost}); !errors.Is(err, session.ErrCWDUnavailable) {
		t.Fatalf("mixed patch err = %v, want ErrCWDUnavailable", err)
	}
	if store.renamed != ([2]string{}) {
		t.Fatalf("invalid mixed patch renamed session: %v", store.renamed)
	}

	missing := "/missing/project"
	if _, err := c.Create(context.Background(), "New", missing); !errors.Is(err, session.ErrCWDUnavailable) {
		t.Fatalf("missing create cwd err = %v, want ErrCWDUnavailable", err)
	}
	if store.createCWD != "" {
		t.Fatalf("missing create cwd wrote session: %q", store.createCWD)
	}
}

// pagedSessionStore seeks the way the store does: (favorite, updatedAt, id)
// descending in favorite and recency, with the id making the order total. A zero
// anchor is the first page, not a position before every row.
type pagedSessionStore struct {
	*crudSessionStore
	rows []session.Session

	afterID string
	limit   int
}

func (s *pagedSessionStore) ListPage(_ context.Context, afterFavorite bool, afterUpdatedAt int64, afterID string, limit int) ([]session.Session, error) {
	s.afterID, s.limit = afterID, limit
	var out []session.Session
	for _, row := range s.rows {
		if afterUpdatedAt != 0 || afterID != "" {
			position := row.UpdatedAt.UnixNano()
			if row.Favorite != afterFavorite || position > afterUpdatedAt || (position == afterUpdatedAt && row.ID <= afterID) {
				continue
			}
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, row)
	}
	return out, nil
}

func sessionRows(ids ...string) []session.Session {
	out := make([]session.Session, 0, len(ids))
	for i, id := range ids {
		out = append(out, session.Session{
			ID: id, CWD: "/repo", UpdatedAt: time.Unix(0, int64(len(ids)-i)).UTC(),
		})
	}
	return out
}

// TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor covers the sessions
// query properties: the order is fixed (favorites first, then recency, id
// last so it is total), the next page seeks strictly past the previous one, and a
// cursor minted by another query is refused rather than restarting from the top —
// which would hand the client sessions it had already read as if they were new.
func TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor(t *testing.T) {
	store := &pagedSessionStore{
		crudSessionStore: &crudSessionStore{},
		rows:             sessionRows("ses_1", "ses_2", "ses_3"),
	}
	c := New(testDependencies(&crudStores{session: store}, Dependencies{
		Paths: testCWDResolver{resolved: "/repo"},
	}))
	ctx := t.Context()

	first, err := c.ListViewPage(ctx, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if store.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", store.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "ses_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two sessions and a cursor", first.Rows)
	}

	second, err := c.ListViewPage(ctx, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if store.afterID != "ses_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", store.afterID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID != "ses_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}

	foreign := pagination.Encode("runs", nil, []string{"1", "0", "ses_1"})
	if _, err := c.ListViewPage(ctx, foreign, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cursor from another query err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListViewPage(ctx, first.NextCursor+"x", 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}
