package session

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestSessionConstruction(t *testing.T) {
	startedAt := time.Unix(1, 0).In(time.FixedZone("fixture", 3600))
	created, err := New(Draft{
		ID: "ses_root", Title: "  Research  ", CWD: "/work/project",
		Model: "  model  ", StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if created.ID() != "ses_root" || created.Title() != "Research" ||
		created.CWD() != "/work/project" || created.Model() != "model" {
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
	valid := Draft{ID: "ses_1", CWD: "/work", StartedAt: time.Unix(1, 0)}
	tests := map[string]Draft{
		"missing identity":   {CWD: valid.CWD, StartedAt: valid.StartedAt},
		"spaced identity":    {ID: " ses_1", CWD: valid.CWD, StartedAt: valid.StartedAt},
		"missing workspace":  {ID: valid.ID, StartedAt: valid.StartedAt},
		"spaced workspace":   {ID: valid.ID, CWD: " /work", StartedAt: valid.StartedAt},
		"missing start time": {ID: valid.ID, CWD: valid.CWD},
	}
	for name, draft := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(draft); !errors.Is(err, ErrInvalid) {
				t.Fatalf("New error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestSessionApplyOwnsNormalizationRevisionAndTime(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	current := mustNew(t, Draft{ID: "ses_1", Title: "Before", CWD: "/work", StartedAt: startedAt})
	title := "  After  "
	model := "  model  "
	favorite := true
	next, changed, err := current.Apply(Patch{
		Title: &title, Model: &model, Favorite: &favorite,
		ExpectedRevision: current.Revision(),
	}, startedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !changed || next.Title() != "After" || next.Model() != "model" || !next.Favorite() {
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
	current := mustNew(t, Draft{ID: "ses_1", Title: "Same", CWD: "/work", StartedAt: startedAt})
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
		ID: "ses_parent", Title: "Research", CWD: "/work/project",
		Model: "model", StartedAt: startedAt,
	})
	parent, _, _ = parent.Apply(Patch{Isolated: &isolated}, startedAt.Add(time.Second))
	childAt := startedAt.Add(2 * time.Second)
	child, err := parent.Fork("ses_child", "", childAt)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ID() != "ses_child" || child.ParentID() != parent.ID() ||
		child.Title() != "Research (fork)" || child.CWD() != parent.CWD() || !child.Isolated() {
		t.Fatalf("child = %+v", child.Snapshot())
	}
	if child.Model() != "" || child.Favorite() || child.Revision() != 1 ||
		!child.StartedAt().Equal(childAt) || !child.UpdatedAt().Equal(childAt) {
		t.Fatalf("child fresh state = %+v", child.Snapshot())
	}
}

func TestSessionGeneratedTitleDoesNotOverrideUserTitle(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	untitled := mustNew(t, Draft{ID: "ses_1", CWD: "/work", StartedAt: startedAt})
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
	current := mustNew(t, Draft{ID: "ses_1", Title: "Current", CWD: "/old", StartedAt: time.Unix(1, 0)})
	restored := mustNew(t, Draft{ID: "ses_1", Title: "Archive", CWD: "/archive", StartedAt: time.Unix(2, 0)})
	restored, err := restored.InstallRestoredWorkspace("/canonical")
	if err != nil {
		t.Fatalf("InstallRestoredWorkspace: %v", err)
	}
	next, err := current.ReplaceWithRestore(restored, time.Unix(3, 0))
	if err != nil {
		t.Fatalf("ReplaceWithRestore: %v", err)
	}
	if next.ID() != current.ID() || next.Title() != "Archive" || next.CWD() != "/canonical" ||
		next.Revision() != current.Revision()+1 || !next.UpdatedAt().Equal(time.Unix(3, 0)) {
		t.Fatalf("restored replacement = %+v", next.Snapshot())
	}
}

func TestSessionRevisionOverflow(t *testing.T) {
	current, err := Restore(Snapshot{
		ID: "ses_1", CWD: "/work", StartedAt: time.Unix(1, 0),
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
	value, err := New(draft)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return value
}
