package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/pagination"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/testsupport/sessionfixture"
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

func (c *crudSessionStore) ListPage(ctx context.Context, _ bool, _ int64, _ string, _ int) ([]session.Session, error) {
	return c.List(ctx)
}

func (c *crudSessionStore) List(context.Context) ([]session.Session, error) { return c.sessions, nil }

func (c *crudSessionStore) Get(_ context.Context, id string) (session.Session, error) {
	c.getID = id
	if c.getErr != nil {
		return session.Session{}, c.getErr
	}
	if c.current.ID() == "" {
		c.current = sessionfixture.MustRestore(session.Snapshot{ID: id, Workspace: sessionfixture.MustWorkspace("/repo")})
	}
	return c.current, nil
}

type generatedTitleRaceStore struct {
	*crudSessionStore
	saveCalls int
	candidate session.Session
}

func (g *generatedTitleRaceStore) Save(
	_ context.Context,
	_ uint64,
	replacement session.Session,
) error {
	g.saveCalls++
	g.candidate = replacement
	userTitle := "User title"
	committed, changed, err := g.current.Apply(
		session.Patch{Title: &userTitle},
		time.Unix(2, 0).UTC(),
	)
	if err != nil || !changed {
		return errors.New("test: could not install concurrent user title")
	}
	g.current = committed
	return session.ErrRevisionConflict
}

func (c *crudSessionStore) Insert(_ context.Context, value session.Session) error {
	c.inserted = value
	c.current = value
	return nil
}

func (c *crudSessionStore) Save(_ context.Context, expected uint64, replacement session.Session) error {
	c.expected = expected
	c.saved = replacement
	if c.saveErr != nil {
		return c.saveErr
	}
	c.current = replacement
	return nil
}

type crudStores struct {
	// session is the port, not the fake: one test pages through a store that seeks,
	// and the harness has no reason to care which fake it is holding.
	session    Store
	interrupts InterruptStore
}

func (c *crudStores) Session() Store { return c.session }
func (c *crudStores) Interrupts() InterruptStore {
	if c.interrupts != nil {
		return c.interrupts
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
		Paths: testWorkspaceResolver{resolved: "/resolved/project"},
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
	if store.getID != "ses_2" || got.Workspace().Path() != "/repo" {
		t.Fatalf("getID=%q got=%+v", store.getID, got)
	}

	created, err := c.Create(ctx, "New", "/requested/project")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID() != "ses_created" || store.inserted.Title() != "New" || store.inserted.Workspace().Path() != "/resolved/project" {
		t.Fatalf("created=%+v inserted=%+v", created.Snapshot(), store.inserted.Snapshot())
	}
}

func TestPrepareScheduledBuildsOneUnpersistedInitialAggregate(t *testing.T) {
	store := &crudSessionStore{getErr: session.ErrNotFound}
	createdAt := time.Unix(9, 0).UTC()
	coordinator := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
		Paths: testWorkspaceResolver{resolved: "/resolved/scheduled"},
		Now:   func() time.Time { return createdAt },
	}))

	current, initial, err := coordinator.PrepareScheduled(
		t.Context(), "ses_scheduled", " Scheduled ", "/requested",
		mustTestSelection(t, "provider", "model"),
	)
	if err != nil {
		t.Fatalf("PrepareScheduled: %v", err)
	}
	if initial == nil || current.Snapshot() != initial.Snapshot() {
		t.Fatalf("scheduled current=%+v initial=%+v", current.Snapshot(), initial)
	}
	if current.ID() != "ses_scheduled" || current.Title() != "Scheduled" ||
		current.Workspace().Path() != "/resolved/scheduled" ||
		current.Selection() != mustTestSelection(t, "provider", "model") ||
		current.Revision() != 1 || !current.StartedAt().Equal(createdAt) {
		t.Fatalf("scheduled aggregate = %+v", current.Snapshot())
	}
	if store.inserted.ID() != "" {
		t.Fatalf("PrepareScheduled persisted before Run opening: %+v", store.inserted.Snapshot())
	}
}

func TestPrepareScheduledReusesCommittedAggregateWithoutWorkspaceAdmission(t *testing.T) {
	existing := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_scheduled", Title: "Existing", Workspace: sessionfixture.MustWorkspace("/existing"),
		Selection: mustTestSelection(t, "provider", "existing-model"),
	})
	store := &crudSessionStore{current: existing}
	coordinator := mustNewCoordinator(testDependencies(&crudStores{session: store}, Dependencies{
		Paths: testWorkspaceResolver{err: errors.New("must not inspect workspace")},
	}))

	current, initial, err := coordinator.PrepareScheduled(
		t.Context(), existing.ID(), "Ignored", "/unavailable",
		mustTestSelection(t, "ignored-provider", "ignored-model"),
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
		ID: "ses_1", Workspace: sessionfixture.MustWorkspace("/repo"), StartedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0),
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

func TestViewPresentsExactSessionSelectionAndWorkspace(t *testing.T) {
	selection, selectionErr := modelref.NewWithReasoningEffort("anthropic", "claude-opus-4-8", "high")
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	c := mustNewCoordinator(Dependencies{
		Paths: testWorkspaceResolver{missing: true}, DefaultModelSelection: selection,
	})

	view, err := c.view(sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_1", Workspace: sessionfixture.MustWorkspace("/repo"), Selection: selection,
	}), ActivityIdle)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Provider != "anthropic" || view.Model != "claude-opus-4-8" || view.ReasoningEffort != "high" {
		t.Fatalf("view selection = %s/%s/%s, want Session selection", view.Provider, view.Model, view.ReasoningEffort)
	}
	if view.Workspace != (WorkspaceView{Path: "/repo", ProjectRoot: "/repo", Missing: true}) {
		t.Fatalf("view workspace = %+v, want one exact missing projection", view.Workspace)
	}
}

func TestCoordinatorUpdateAppliesPatch(t *testing.T) {
	store := &crudSessionStore{}
	claims := new(testClaimer)
	stores := &crudStores{session: store}
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testWorkspaceResolver{resolved: "/resolved/project"}, Admissions: claims}))
	ctx := context.Background()

	title := "  Renamed  "
	selection := mustTestSelection(t, "anthropic", "claude-opus-4-8")
	provider, model, effort := selection.Provider(), selection.Model(), "high"
	selection, selectionErr := modelref.NewWithReasoningEffort(provider, model, effort)
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	cwd := "/requested/project"
	favorite := true

	got, err := c.Update(ctx, "ses_1", Patch{
		Title: &title,
		ModelSelection: modelref.Patch{
			Provider: &provider, Model: &model, ReasoningEffort: &effort,
		},
		WorkspacePath: &cwd,
		Favorite:      &favorite,
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
	if store.saved.Selection() != selection {
		t.Fatalf("model selection = %v", store.saved.Selection())
	}
	if store.saved.Workspace().Path() != "/resolved/project" {
		t.Fatalf("cwd = %q", store.saved.Workspace().Path())
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
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testWorkspaceResolver{resolved: "/resolved/project"}, Admissions: claims}))
	cwd := "/requested/project"

	_, err := c.Update(t.Context(), "ses_1", Patch{WorkspacePath: &cwd})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Update relocation error = %v, want ErrSessionBusy", err)
	}
	if store.saved.ID() != "" {
		t.Fatal("busy relocation mutated the session")
	}
}

func TestCoordinatorUpdateRejectsExecutionPolicyChangeWhileParked(t *testing.T) {
	for name, patch := range map[string]Patch{
		"workspace": func() Patch {
			cwd := "/requested/project"
			return Patch{WorkspacePath: &cwd}
		}(),
		"isolation": func() Patch {
			isolated := true
			return Patch{Isolated: &isolated}
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
				Paths:      testWorkspaceResolver{resolved: "/resolved/project"},
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
	c := mustNewCoordinator(testDependencies(stores, Dependencies{Paths: testWorkspaceResolver{err: errors.New("cwd unavailable")}}))

	blank := "  "
	if _, err := c.Update(t.Context(), "ses_1", Patch{Title: &blank}); !errors.Is(err, session.ErrTitleRequired) {
		t.Fatalf("blank title err = %v, want ErrTitleRequired", err)
	}
	if store.saved.ID() != "" {
		t.Fatalf("blank title saved Session: %+v", store.saved.Snapshot())
	}

	ghost := "/no/such/dir"
	if _, err := c.Update(t.Context(), "ses_1", Patch{WorkspacePath: &ghost}); !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("ghost cwd err = %v, want ErrCWDUnavailable", err)
	}
	if store.saved.ID() != "" {
		t.Fatalf("ghost cwd saved Session: %+v", store.saved.Snapshot())
	}

	title := "Renamed"
	if _, err := c.Update(t.Context(), "ses_1", Patch{Title: &title, WorkspacePath: &ghost}); !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
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

func (p *pagedSessionStore) ListPage(_ context.Context, afterFavorite bool, afterUpdatedAt int64, afterID string, limit int) ([]session.Session, error) {
	p.afterID, p.limit = afterID, limit
	var out []session.Session
	for _, row := range p.rows {
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
			ID: id, Workspace: sessionfixture.MustWorkspace("/repo"), StartedAt: updatedAt, UpdatedAt: updatedAt,
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
		Paths: testWorkspaceResolver{resolved: "/repo"},
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
