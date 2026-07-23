package skillauthoring_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

const (
	draftSubdir   = "_drafts"
	archiveSubdir = "_archive"
)

func TestSaveDraftThenPromote(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	draft := skills.Draft{
		Name:        "git-bisect-helper",
		Description: "Walk a git bisect to find a regression; use it when a test started failing.",
		Body:        "# Steps\n1. `git bisect start`\n2. mark good/bad\n",
	}

	handle, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	// The draft is under _drafts (invisible to the read-only source) — not active.
	if _, err := os.Stat(filepath.Join(root, "_drafts", handle.Revision, "SKILL.md")); err != nil {
		t.Fatalf("draft not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, draft.Name, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("an unpromoted draft must not appear in the active set")
	}

	if err := store.Promote(t.Context(), handle); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	// Now active, and the draft is gone.
	if _, err := os.Stat(filepath.Join(root, "_drafts", handle.Revision)); !os.IsNotExist(err) {
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
	handle, err := store.SaveDraft(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(t.Context(), handle); err != nil {
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
	if err := store.Archive(t.Context(), "alpha-skill"); err != nil {
		t.Fatalf("replayed Archive: %v", err)
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
	if err := store.Restore(t.Context(), "alpha-skill"); err != nil {
		t.Fatalf("replayed Restore: %v", err)
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
	handle := skills.NewDraftHandle("never-proposed", []byte("missing"))
	if err := store.Promote(t.Context(), handle); err == nil {
		t.Fatal("promoting a nonexistent draft must error")
	}
}

func TestDiscardDraft(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	draft := skills.Draft{Name: "throwaway", Description: "A description long enough to pass validation.", Body: "body"}
	handle, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DiscardDraft(t.Context(), handle); err != nil {
		t.Fatalf("DiscardDraft: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_drafts", handle.Revision)); !os.IsNotExist(err) {
		t.Fatal("discard should remove the draft dir")
	}
}

func TestSaveDraftRejectsInvalid(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	// Invalid name (uppercase / spaces) is refused before anything is written.
	if _, err := store.SaveDraft(t.Context(), skills.Draft{Name: "Bad Name", Description: "desc that is long enough", Body: "b"}); err == nil {
		t.Fatal("invalid skill name must be rejected")
	}
}

func TestSameNameDraftsKeepIndependentApprovedBytes(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	first := skills.Draft{Name: "shared-name", Description: "The first independently approved skill version.", Body: "first body"}
	second := skills.Draft{Name: "shared-name", Description: "The second independently approved skill version.", Body: "second body"}

	firstHandle, err := store.SaveDraft(t.Context(), first)
	if err != nil {
		t.Fatalf("save first draft: %v", err)
	}
	secondHandle, err := store.SaveDraft(t.Context(), second)
	if err != nil {
		t.Fatalf("save second draft: %v", err)
	}
	if firstHandle == secondHandle {
		t.Fatal("different proposal bytes received the same handle")
	}
	if err := store.DiscardDraft(t.Context(), firstHandle); err != nil {
		t.Fatalf("discard first draft: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, draftSubdir, secondHandle.Revision, "SKILL.md")); err != nil {
		t.Fatalf("discarding one proposal removed another: %v", err)
	}
	if err := store.Promote(t.Context(), secondHandle); err != nil {
		t.Fatalf("promote second draft: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, second.Name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read active skill: %v", err)
	}
	if !secondHandle.Matches(content) {
		t.Fatal("promoted bytes do not match the approved handle")
	}
}

func TestPromoteRejectsChangedDraftWithoutTouchingActiveSet(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	draft := skills.Draft{Name: "immutable-draft", Description: "Verify immutable proposal publication semantics.", Body: "approved body"}
	handle, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	path := filepath.Join(root, draftSubdir, handle.Revision, "SKILL.md")
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper draft: %v", err)
	}

	if err := store.Promote(t.Context(), handle); !errors.Is(err, skills.ErrDraftChanged) {
		t.Fatalf("Promote() error = %v, want ErrDraftChanged", err)
	}
	if _, err := os.Stat(filepath.Join(root, draft.Name)); !os.IsNotExist(err) {
		t.Fatal("a changed draft reached the active set")
	}
	if err := store.DiscardDraft(t.Context(), handle); !errors.Is(err, skills.ErrDraftChanged) {
		t.Fatalf("DiscardDraft() error = %v, want ErrDraftChanged", err)
	}
}

func TestPromoteIsIdempotentForExactReplay(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	draft := skills.Draft{Name: "replay-safe", Description: "Make suspended proposal replay deterministic.", Body: "same body"}
	first, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatalf("first SaveDraft: %v", err)
	}
	second, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatalf("second SaveDraft: %v", err)
	}
	if first != second {
		t.Fatalf("replayed handles differ: %+v vs %+v", first, second)
	}
	if err := store.Promote(t.Context(), first); err != nil {
		t.Fatalf("first Promote: %v", err)
	}
	if _, err := store.SaveDraft(t.Context(), draft); err != nil {
		t.Fatalf("restage replay: %v", err)
	}
	if err := store.Promote(t.Context(), first); err != nil {
		t.Fatalf("replayed Promote: %v", err)
	}
}

func TestLifecycleConflictsPreserveBothStates(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	promote(t, store, "conflict-safe")
	if err := store.Archive(t.Context(), "conflict-safe"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	draft := skills.Draft{Name: "conflict-safe", Description: "A different version must not replace the archive.", Body: "replacement"}
	handle, err := store.SaveDraft(t.Context(), draft)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := store.Promote(t.Context(), handle); !errors.Is(err, skills.ErrConflict) {
		t.Fatalf("Promote() error = %v, want ErrConflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, archiveSubdir, draft.Name, "SKILL.md")); err != nil {
		t.Fatalf("archived version was lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, draftSubdir, handle.Revision, "SKILL.md")); err != nil {
		t.Fatalf("conflicting draft was lost: %v", err)
	}
}

func TestConcurrentPromotionsPublishOneRevisionWithoutLosingTheOther(t *testing.T) {
	root := t.TempDir()
	stores := []*skillauthoring.Store{skillauthoring.NewStore(root), skillauthoring.NewStore(root)}
	drafts := []skills.Draft{
		{Name: "ordered-publish", Description: "The first concurrently proposed skill revision.", Body: "first"},
		{Name: "ordered-publish", Description: "The second concurrently proposed skill revision.", Body: "second"},
	}
	handles := make([]skills.DraftHandle, len(drafts))
	for i, draft := range drafts {
		handle, err := stores[i].SaveDraft(t.Context(), draft)
		if err != nil {
			t.Fatalf("SaveDraft(%d): %v", i, err)
		}
		handles[i] = handle
	}

	errs := make([]error, len(handles))
	var wait sync.WaitGroup
	for i, handle := range handles {
		wait.Go(func() { errs[i] = stores[i].Promote(t.Context(), handle) })
	}
	wait.Wait()

	succeeded, conflicted := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, skills.ErrConflict):
			conflicted++
		default:
			t.Fatalf("unexpected promotion error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("promotion outcomes = %d success, %d conflict; want 1 and 1", succeeded, conflicted)
	}

	active, err := os.ReadFile(filepath.Join(root, drafts[0].Name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read active skill: %v", err)
	}
	winner := -1
	for i, handle := range handles {
		if handle.Matches(active) {
			winner = i
			break
		}
	}
	if winner < 0 {
		t.Fatal("active bytes match neither proposed revision")
	}
	loser := 1 - winner
	if _, err := os.Stat(filepath.Join(root, draftSubdir, handles[loser].Revision, "SKILL.md")); err != nil {
		t.Fatalf("losing revision was destroyed: %v", err)
	}
}
