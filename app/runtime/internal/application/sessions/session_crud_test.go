package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/pagination"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

type crudSessionStore struct {
	sessions []session.Session
	current  session.Session
	getErr   error
	getID    string
	inserted session.Session
	saved    session.Session
	expected uint64
	saveErr  error
}

func (s *crudSessionStore) ListPage(ctx context.Context, _ bool, _ int64, _ string, _ int) ([]session.Session, error) {
	return s.List(ctx)
}

func (s *crudSessionStore) List(context.Context) ([]session.Session, error) { return s.sessions, nil }

func (s *crudSessionStore) Get(_ context.Context, id string) (session.Session, error) {
	s.getID = id
	if s.getErr != nil {
		return session.Session{}, s.getErr
	}
	if s.current.ID() == "" {
		s.current = sessionfixture.MustRestore(session.Snapshot{ID: id, CWD: "/repo"})
	}
	return s.current, nil
}

type generatedTitleRaceStore struct {
	*crudSessionStore
	saveCalls int
	candidate session.Session
}

func (s *generatedTitleRaceStore) Save(
	_ context.Context,
	_ uint64,
	replacement session.Session,
) error {
	s.saveCalls++
	s.candidate = replacement
	userTitle := "User title"
	committed, changed, err := s.current.Apply(
		session.Patch{Title: &userTitle},
		time.Unix(2, 0).UTC(),
	)
	if err != nil || !changed {
		return errors.New("test: could not install concurrent user title")
	}
	s.current = committed
	return session.ErrRevisionConflict
}

func (s *crudSessionStore) Insert(_ context.Context, value session.Session) error {
	s.inserted = value
	s.current = value
	return nil
}

func (s *crudSessionStore) Save(_ context.Context, expected uint64, replacement session.Session) error {
	s.expected = expected
	s.saved = replacement
	if s.saveErr != nil {
		return s.saveErr
	}
	s.current = replacement
	return nil
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
	store := &crudSessionStore{sessions: []session.Session{
		sessionfixture.MustRestore(session.Snapshot{ID: "ses_1"}),
	}}
	stores := &crudStores{session: store}
	c := mustNewCoordinator(testDependencies(stores, Dependencies{
		Paths: testCWDResolver{resolved: "/resolved/project"},
		Now:   func() time.Time { return time.Unix(2, 0).UTC() },
		NewID: func() string { return "ses_created" },
	}))
	ctx := context.Background()

	listed, err := c.List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID() != "ses_1" {
		t.Fatalf("listed = %+v", listed)
	}

	got, err := c.Get(ctx, "ses_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if store.getID != "ses_2" || got.CWD() != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	created, err := c.Create(ctx, "New", "/requested/project")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID() != "ses_created" || store.inserted.Title() != "New" || store.inserted.CWD() != "/resolved/project" {
		t.Fatalf("created=%+v inserted=%+v", created.Snapshot(), store.inserted.Snapshot())
	}
}

func TestPrepareScheduledBuildsOneUnpersistedInitialAggregate(t *testing.T) {
	store := &crudSessionStore{getErr: session.ErrNotFound}
	createdAt := time.Unix(9, 0).UTC()
	coordinator := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
		Paths: testCWDResolver{resolved: "/resolved/scheduled"},
		Now:   func() time.Time { return createdAt },
	}))

	current, initial, err := coordinator.PrepareScheduled(
		t.Context(), "ses_scheduled", " Scheduled ", "/requested", " model ",
	)
	if err != nil {
		t.Fatalf("PrepareScheduled: %v", err)
	}
	if initial == nil || current.Snapshot() != initial.Snapshot() {
		t.Fatalf("scheduled current=%+v initial=%+v", current.Snapshot(), initial)
	}
	if current.ID() != "ses_scheduled" || current.Title() != "Scheduled" ||
		current.CWD() != "/resolved/scheduled" || current.Model() != "model" ||
		current.Revision() != 1 || !current.StartedAt().Equal(createdAt) {
		t.Fatalf("scheduled aggregate = %+v", current.Snapshot())
	}
	if store.inserted.ID() != "" {
		t.Fatalf("PrepareScheduled persisted before Run opening: %+v", store.inserted.Snapshot())
	}
}

func TestPrepareScheduledReusesCommittedAggregateWithoutWorkspaceAdmission(t *testing.T) {
	existing := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_scheduled", Title: "Existing", CWD: "/existing", Model: "existing-model",
	})
	store := &crudSessionStore{current: existing}
	coordinator := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
		Paths: testCWDResolver{err: errors.New("must not inspect workspace")},
	}))

	current, initial, err := coordinator.PrepareScheduled(
		t.Context(), existing.ID(), "Ignored", "/unavailable", "ignored-model",
	)
	if err != nil {
		t.Fatalf("PrepareScheduled existing: %v", err)
	}
	if initial != nil || current.Snapshot() != existing.Snapshot() {
		t.Fatalf("existing current=%+v initial=%+v", current.Snapshot(), initial)
	}
}

func TestGeneratedTitleLosesToConcurrentUserTitle(t *testing.T) {
	current := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_1", CWD: "/repo", StartedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0),
	})
	store := &generatedTitleRaceStore{crudSessionStore: &crudSessionStore{current: current}}
	coordinator := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
		Now: func() time.Time { return time.Unix(3, 0).UTC() },
	}))

	if err := coordinator.ApplyGeneratedTitle(t.Context(), current.ID(), "Generated title"); err != nil {
		t.Fatalf("ApplyGeneratedTitle: %v", err)
	}
	if store.saveCalls != 1 || store.candidate.Title() != "Generated title" {
		t.Fatalf("generated attempt calls=%d candidate=%+v", store.saveCalls, store.candidate.Snapshot())
	}
	if store.current.Title() != "User title" || store.current.Revision() != 2 {
		t.Fatalf("concurrent user title lost: %+v", store.current.Snapshot())
	}
}

func TestViewUsesConfiguredDefaultModel(t *testing.T) {
	c := mustNewCoordinator(Dependencies{Paths: testCWDResolver{}, DefaultModel: "claude-opus-4-8"})

	view, err := c.view(sessionfixture.MustRestore(session.Snapshot{ID: "ses_1", CWD: "/repo"}), ActivityIdle)
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
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testCWDResolver{resolved: "/resolved/project"}, Admissions: claims}))
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
	if store.saved.ID() == "" {
		t.Fatal("Update did not save the decided replacement")
	}
	if got.ID() != "ses_1" || store.saved.Title() != "Renamed" {
		t.Fatalf("updated=%+v saved=%+v", got.Snapshot(), store.saved.Snapshot())
	}
	if store.saved.Model() != model {
		t.Fatalf("model = %q", store.saved.Model())
	}
	if store.saved.CWD() != "/resolved/project" {
		t.Fatalf("cwd = %q", store.saved.CWD())
	}
	if !store.saved.Favorite() || store.expected != 1 || store.saved.Revision() != 2 {
		t.Fatalf("saved lifecycle = %+v, expected=%d", store.saved.Snapshot(), store.expected)
	}
	if len(claims.released) != 1 || claims.released[0] != "ses_1" {
		t.Fatalf("relocation admission releases = %v, want [ses_1]", claims.released)
	}
}

func TestCoordinatorUpdateRejectsRelocationDuringRun(t *testing.T) {
	store := &crudSessionStore{}
	claims := &testClaimer{claimed: map[string]bool{"ses_1": true}}
	stores := &crudStores{session: store}
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testCWDResolver{resolved: "/resolved/project"}, Admissions: claims}))
	cwd := "/requested/project"

	_, err := c.Update(t.Context(), "ses_1", session.Patch{CWD: &cwd})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Update relocation error = %v, want ErrSessionBusy", err)
	}
	if store.saved.ID() != "" {
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
			coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
				Paths:      testCWDResolver{resolved: "/resolved/project"},
				Admissions: new(testClaimer),
			}))

			_, err := coordinator.Update(t.Context(), "ses_1", patch)
			if !errors.Is(err, ErrSessionBusy) {
				t.Fatalf("Update error = %v, want ErrSessionBusy", err)
			}
			if store.saved.ID() != "" {
				t.Fatal("parked execution policy change mutated the Session")
			}
		})
	}
}

func TestCoordinatorUpdateRejectsInvalidPatch(t *testing.T) {
	store := &crudSessionStore{}
	stores := &crudStores{session: store}
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testCWDResolver{err: errors.New("cwd unavailable")}}))

	blank := "  "
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.saved.ID() != "" {
		t.Fatalf("blank title saved Session: %+v", store.saved.Snapshot())
	}

	ghost := "/no/such/dir"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{CWD: &ghost}); !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCWDUnavailable", err)
	}
	if store.saved.ID() != "" {
		t.Fatalf("ghost cwd saved Session: %+v", store.saved.Snapshot())
	}

	title := "Renamed"
	if _, err := c.Update(t.Context(), "ses_1", session.Patch{Title: &title, CWD: &ghost}); !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("mixed patch err = %v, want ErrCWDUnavailable", err)
	}
	if store.saved.ID() != "" {
		t.Fatalf("invalid mixed patch saved Session: %+v", store.saved.Snapshot())
	}

	missing := "/missing/project"
	if _, err := c.Create(context.Background(), "New", missing); !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("missing create cwd err = %v, want ErrCWDUnavailable", err)
	}
	if store.inserted.ID() != "" {
		t.Fatalf("missing create cwd inserted Session: %+v", store.inserted.Snapshot())
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
			position := row.UpdatedAt().UnixNano()
			if row.Favorite() != afterFavorite || position > afterUpdatedAt || (position == afterUpdatedAt && row.ID() <= afterID) {
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
		updatedAt := time.Unix(0, int64(len(ids)-i)).UTC()
		out = append(out, sessionfixture.MustRestore(session.Snapshot{
			ID: id, CWD: "/repo", StartedAt: updatedAt, UpdatedAt: updatedAt,
		}))
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
	c := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
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
