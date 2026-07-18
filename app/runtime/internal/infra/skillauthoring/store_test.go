package skillauthoring_test

import (
	"os"
	"path/filepath"
	"testing"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

func TestSaveDraftThenPromote(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	draft := skills.Draft{
		Name:        "git-bisect-helper",
		Description: "Walk a git bisect to find a regression; use it when a test started failing.",
		Body:        "# Steps\n1. `git bisect start`\n2. mark good/bad\n",
	}

	if err := store.SaveDraft(t.Context(), draft); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	// The draft is under _drafts (invisible to the read-only source) — not active.
	if _, err := os.Stat(filepath.Join(root, "_drafts", draft.Name, "SKILL.md")); err != nil {
		t.Fatalf("draft not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, draft.Name, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("an unpromoted draft must not appear in the active set")
	}

	if err := store.Promote(t.Context(), draft.Name); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	// Now active, and the draft is gone.
	if _, err := os.Stat(filepath.Join(root, "_drafts", draft.Name)); !os.IsNotExist(err) {
		t.Fatal("promotion should remove the draft")
	}

	// The promoted skill is discoverable + valid per the read-only spec loader.
	source := skillspec.Dir(root)
	summaries, err := source.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Name != draft.Name {
		t.Fatalf("promoted skill not discoverable: %+v", summaries)
	}
	loaded, err := source.Load(t.Context(), draft.Name)
	if err != nil {
		t.Fatalf("Load promoted skill: %v", err)
	}
	if loaded.Description != draft.Description {
		t.Fatalf("loaded description = %q, want %q", loaded.Description, draft.Description)
	}
}

func promote(t *testing.T, store *skillauthoring.Store, name string) {
	t.Helper()
	d := skills.Draft{Name: name, Description: "A description that is long enough to validate.", Body: "do the thing"}
	if err := store.SaveDraft(t.Context(), d); err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(t.Context(), name); err != nil {
		t.Fatal(err)
	}
}

func lifecycleOf(entries []skills.Entry, name string) (skills.Lifecycle, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e.Lifecycle, true
		}
	}
	return "", false
}

func TestArchiveRestoreAndList(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	promote(t, store, "alpha-skill")
	promote(t, store, "beta-skill")

	// Both active.
	list, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if lc, ok := lifecycleOf(list, "alpha-skill"); !ok || lc != skills.Active {
		t.Fatalf("alpha should be active, got %q (%v)", lc, ok)
	}

	// Archive alpha → it leaves the active set, still listed as archived, and is
	// no longer discovered by the read-only source (not loadable).
	if err := store.Archive(t.Context(), "alpha-skill"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	list, _ = store.List(t.Context())
	if lc, _ := lifecycleOf(list, "alpha-skill"); lc != skills.Archived {
		t.Fatalf("alpha should be archived, got %q", lc)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha-skill", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("archived skill must leave the active directory")
	}
	if _, err := os.Stat(filepath.Join(root, "_archive", "alpha-skill", "SKILL.md")); err != nil {
		t.Fatalf("archived skill must be preserved under _archive: %v", err)
	}

	// Restore → active again.
	if err := store.Restore(t.Context(), "alpha-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	list, _ = store.List(t.Context())
	if lc, _ := lifecycleOf(list, "alpha-skill"); lc != skills.Active {
		t.Fatalf("restored alpha should be active, got %q", lc)
	}
}

func TestArchiveMissingErrors(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	if err := store.Archive(t.Context(), "nope"); err == nil {
		t.Fatal("archiving a nonexistent skill must error")
	}
	if err := store.Restore(t.Context(), "nope"); err == nil {
		t.Fatal("restoring a nonexistent archived skill must error")
	}
}

func TestPromoteMissingDraftErrors(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	if err := store.Promote(t.Context(), "never-proposed"); err == nil {
		t.Fatal("promoting a nonexistent draft must error")
	}
}

func TestDiscardDraft(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	draft := skills.Draft{Name: "throwaway", Description: "A description long enough to pass validation.", Body: "body"}
	if err := store.SaveDraft(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	if err := store.DiscardDraft(t.Context(), draft.Name); err != nil {
		t.Fatalf("DiscardDraft: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_drafts", draft.Name)); !os.IsNotExist(err) {
		t.Fatal("discard should remove the draft dir")
	}
}

func TestSaveDraftRejectsInvalid(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	// Invalid name (uppercase / spaces) is refused before anything is written.
	if err := store.SaveDraft(t.Context(), skills.Draft{Name: "Bad Name", Description: "desc that is long enough", Body: "b"}); err == nil {
		t.Fatal("invalid skill name must be rejected")
	}
}
