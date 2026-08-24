package session

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func TestSessionOwnsExactSelectionAcrossEditAndFork(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	initial, err := modelref.New("provider-a", "shared-model")
	if err != nil {
		t.Fatalf("initial selection: %v", err)
	}
	parent := mustNew(t, Draft{
		ID: "ses_parent", Workspace: mustWorkspace(t, "/work"), Selection: initial, StartedAt: startedAt,
	})
	if parent.Selection() != initial {
		t.Fatalf("initial selection = %v, want %v", parent.Selection(), initial)
	}

	replacement, err := modelref.New("provider-b", "shared-model")
	if err != nil {
		t.Fatalf("replacement selection: %v", err)
	}
	next, changed, err := parent.Apply(Patch{Selection: &replacement}, startedAt.Add(time.Second))
	if err != nil || !changed || next.Selection() != replacement {
		t.Fatalf("selection replacement = %v, changed=%v, err=%v", next.Selection(), changed, err)
	}
	child, err := next.Fork("ses_child", "", startedAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.Selection() != replacement {
		t.Fatalf("fork selection = %v, want %v", child.Selection(), replacement)
	}
}

func TestSessionConstruction(t *testing.T) {
	startedAt := time.Unix(1, 0).In(time.FixedZone("fixture", 3600))
	selection := mustModelSelection(t, "provider", "model")
	created, err := New(Draft{
		ID: "ses_root", Title: "  Research  ", Workspace: mustWorkspace(t, "/work/project"),
		Selection: selection, StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if created.ID() != "ses_root" || created.Title() != "Research" ||
		created.Workspace().Path() != "/work/project" || created.Selection() != selection {
		t.Fatalf("created Session = %+v", created.Snapshot())
	}
	if created.ParentID() != "" || created.Revision() != 1 ||
		!created.StartedAt().Equal(startedAt) || !created.UpdatedAt().Equal(startedAt) {
		t.Fatalf("created lifecycle = %+v", created.Snapshot())
	}
	if created.StartedAt().Location() != time.UTC || created.UpdatedAt().Location() != time.UTC {
		t.Fatal("construction did not canonicalize times to UTC")
	}
}

func TestSessionConstructionRejectsInvalidState(t *testing.T) {
	selection := mustModelSelection(t, "provider", "model")
	valid := Draft{ID: "ses_1", Workspace: mustWorkspace(t, "/work"), Selection: selection, StartedAt: time.Unix(1, 0)}
	tests := map[string]Draft{
		"missing identity":   {Workspace: valid.Workspace, Selection: selection, StartedAt: valid.StartedAt},
		"spaced identity":    {ID: " ses_1", Workspace: valid.Workspace, Selection: selection, StartedAt: valid.StartedAt},
		"missing workspace":  {ID: valid.ID, Selection: selection, StartedAt: valid.StartedAt},
		"missing selection":  {ID: valid.ID, Workspace: valid.Workspace, StartedAt: valid.StartedAt},
		"missing start time": {ID: valid.ID, Workspace: valid.Workspace, Selection: selection},
	}
	for name, draft := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(draft); !errors.Is(err, ErrInvalid) {
				t.Fatalf("New error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestWorkspaceRejectsNonExactPaths(t *testing.T) {
	for name, path := range map[string]string{
		"missing": "", "relative": "relative/work", "unclean": "/work/../work",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewWorkspace(path); !errors.Is(err, ErrInvalid) {
				t.Fatalf("NewWorkspace(%q) error = %v, want ErrInvalid", path, err)
			}
		})
	}
	if root, err := NewWorkspace("/"); err != nil || root.Path() != "/" {
		t.Fatalf("root workspace = %q, err=%v", root.Path(), err)
	}
	if spaced, err := NewWorkspace("/work "); err != nil || spaced.Path() != "/work " {
		t.Fatalf("workspace with significant space = %q, err=%v", spaced.Path(), err)
	}
}

func TestSessionApplyOwnsNormalizationRevisionAndTime(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	current := mustNew(t, Draft{ID: "ses_1", Title: "Before", Workspace: mustWorkspace(t, "/work"), StartedAt: startedAt})
	title := "  After  "
	selection := mustModelSelection(t, "provider", "model")
	favorite := true
	next, changed, err := current.Apply(Patch{
		Title: &title, Selection: &selection, Favorite: &favorite,
		ExpectedRevision: current.Revision(),
	}, startedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !changed || next.Title() != "After" || next.Selection() != selection || !next.Favorite() {
		t.Fatalf("replacement = %+v, changed=%v", next.Snapshot(), changed)
	}
	if next.Revision() != current.Revision()+1 || !next.UpdatedAt().Equal(startedAt.Add(time.Second)) {
		t.Fatalf("replacement lifecycle = %+v", next.Snapshot())
	}
	if current.Title() != "Before" || current.Revision() != 1 {
		t.Fatalf("source mutated = %+v", current.Snapshot())
	}
}

func TestSessionApplyNoopAndConflicts(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	current := mustNew(t, Draft{ID: "ses_1", Title: "Same", Workspace: mustWorkspace(t, "/work"), StartedAt: startedAt})
	title := " Same "
	unmoved, changed, err := current.Apply(Patch{Title: &title}, startedAt.Add(time.Second))
	if err != nil || changed || unmoved.Snapshot() != current.Snapshot() {
		t.Fatalf("semantic no-op = %+v, changed=%v, err=%v", unmoved.Snapshot(), changed, err)
	}
	if _, _, err := current.Apply(Patch{ExpectedRevision: 2}, startedAt); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v, want ErrRevisionConflict", err)
	}
	blank := "  "
	if _, _, err := current.Apply(Patch{Title: &blank}, startedAt); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("blank title error = %v, want ErrTitleRequired", err)
	}
	other := "Other"
	if _, _, err := current.Apply(Patch{Title: &other}, startedAt.Add(-time.Second)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("time travel error = %v, want ErrInvalid", err)
	}
}

func TestSessionFork(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	isolated := true
	parent := mustNew(t, Draft{
		ID: "ses_parent", Title: "Research", Workspace: mustWorkspace(t, "/work/project"),
		Selection: mustModelSelection(t, "provider", "model"), StartedAt: startedAt,
	})
	parent, _, _ = parent.Apply(Patch{Isolated: &isolated}, startedAt.Add(time.Second))
	childAt := startedAt.Add(2 * time.Second)
	child, err := parent.Fork("ses_child", "", childAt)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ID() != "ses_child" || child.ParentID() != parent.ID() ||
		child.Title() != "Research (fork)" || child.Workspace() != parent.Workspace() || !child.Isolated() {
		t.Fatalf("child = %+v", child.Snapshot())
	}
	if child.Selection() != parent.Selection() || child.Favorite() || child.Revision() != 1 ||
		!child.StartedAt().Equal(childAt) || !child.UpdatedAt().Equal(childAt) {
		t.Fatalf("child fresh state = %+v", child.Snapshot())
	}
}

func TestSessionGeneratedTitleDoesNotOverrideUserTitle(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	untitled := mustNew(t, Draft{ID: "ses_1", Workspace: mustWorkspace(t, "/work"), StartedAt: startedAt})
	named, changed, err := untitled.NameIfUntitled(" Generated ", startedAt.Add(time.Second))
	if err != nil || !changed || named.Title() != "Generated" {
		t.Fatalf("generated title = %+v, changed=%v, err=%v", named.Snapshot(), changed, err)
	}
	unmoved, changed, err := named.NameIfUntitled("Replacement", startedAt.Add(2*time.Second))
	if err != nil || changed || unmoved.Snapshot() != named.Snapshot() {
		t.Fatalf("named Session changed = %+v, changed=%v, err=%v", unmoved.Snapshot(), changed, err)
	}
}

func TestSessionRestoreReplacementKeepsTargetRevisionSpace(t *testing.T) {
	current := mustNew(t, Draft{ID: "ses_1", Title: "Current", Workspace: mustWorkspace(t, "/old"), StartedAt: time.Unix(1, 0)})
	restored := mustNew(t, Draft{ID: "ses_1", Title: "Archive", Workspace: mustWorkspace(t, "/archive"), StartedAt: time.Unix(2, 0)})
	restored, err := restored.InstallRestoredWorkspace(mustWorkspace(t, "/canonical"))
	if err != nil {
		t.Fatalf("InstallRestoredWorkspace: %v", err)
	}
	next, err := current.ReplaceWithRestore(restored, time.Unix(3, 0))
	if err != nil {
		t.Fatalf("ReplaceWithRestore: %v", err)
	}
	if next.ID() != current.ID() || next.Title() != "Archive" || next.Workspace().Path() != "/canonical" ||
		next.Revision() != current.Revision()+1 || !next.UpdatedAt().Equal(time.Unix(3, 0)) {
		t.Fatalf("restored replacement = %+v", next.Snapshot())
	}
}

func TestSessionRevisionOverflow(t *testing.T) {
	current, err := Restore(Snapshot{
		ID: "ses_1", Workspace: mustWorkspace(t, "/work"), StartedAt: time.Unix(1, 0),
		Selection: mustModelSelection(t, "provider", "model"),
		UpdatedAt: time.Unix(1, 0), Revision: math.MaxUint64,
	})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	title := "Changed"
	if _, _, err := current.Apply(Patch{Title: &title}, time.Unix(2, 0)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overflow error = %v, want ErrInvalid", err)
	}
}

func mustNew(t *testing.T, draft Draft) Session {
	t.Helper()
	if !draft.Selection.Configured() {
		draft.Selection = mustModelSelection(t, "test-provider", "test-model")
	}
	value, err := New(draft)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return value
}

func mustModelSelection(t *testing.T, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New: %v", err)
	}
	return selection
}

func mustWorkspace(t *testing.T, path string) Workspace {
	t.Helper()
	workspace, err := NewWorkspace(path)
	if err != nil {
		t.Fatalf("NewWorkspace(%q): %v", path, err)
	}
	return workspace
}
