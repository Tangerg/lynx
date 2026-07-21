package skillauthoring_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// installActive promotes a plain (non-revising) draft so a named skill is active.
func installActive(t *testing.T, store *skillauthoring.Store, name, body string) {
	t.Helper()
	handle, err := store.SaveDraft(t.Context(), skills.Draft{
		Name:        name,
		Description: "A skill with a description long enough to validate.",
		Body:        body,
	})
	if err != nil {
		t.Fatalf("SaveDraft(%s): %v", name, err)
	}
	if err := store.Promote(t.Context(), handle); err != nil {
		t.Fatalf("Promote(%s): %v", name, err)
	}
}

// promoteRevision saves + promotes a revising draft for name with a new body.
func promoteRevision(t *testing.T, store *skillauthoring.Store, name, body string) {
	t.Helper()
	handle, err := store.SaveDraft(t.Context(), skills.Draft{
		Name:        name,
		Description: "A skill with a description long enough to validate.",
		Body:        body,
		CreatedBy:   skills.CreatedByAgent,
		Revises:     true,
	})
	if err != nil {
		t.Fatalf("SaveDraft(revision %s): %v", name, err)
	}
	if err := store.Promote(t.Context(), handle); err != nil {
		t.Fatalf("Promote(revision %s): %v", name, err)
	}
}

func TestListDraftsReportsHandlesAndProvenance(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())

	mined := skills.Draft{
		Name:          "run-project-tests",
		Description:   "Run the module test suite. Use when asked to run or verify tests.",
		Body:          "Run `go test ./...` from the module root.",
		CreatedBy:     skills.CreatedByAgent,
		SourceSession: "ses_42",
	}
	authored := skills.Draft{
		Name:        "manual-note",
		Description: "A draft with no provenance, as a human proposal would carry.",
		Body:        "do the thing",
	}
	minedHandle, err := store.SaveDraft(t.Context(), mined)
	if err != nil {
		t.Fatalf("SaveDraft(mined): %v", err)
	}
	if _, err := store.SaveDraft(t.Context(), authored); err != nil {
		t.Fatalf("SaveDraft(authored): %v", err)
	}

	drafts, err := store.ListDrafts(t.Context())
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("ListDrafts returned %d drafts, want 2", len(drafts))
	}

	byName := map[string]skills.DraftInfo{}
	for _, d := range drafts {
		byName[d.Handle.Name] = d
	}
	got := byName["run-project-tests"]
	if got.Handle != minedHandle {
		t.Errorf("mined handle = %+v, want %+v", got.Handle, minedHandle)
	}
	if got.Description != mined.Description {
		t.Errorf("mined description = %q", got.Description)
	}
	if got.CreatedBy != skills.CreatedByAgent {
		t.Errorf("mined CreatedBy = %q, want %q", got.CreatedBy, skills.CreatedByAgent)
	}
	if got.SourceSession != "ses_42" {
		t.Errorf("mined SourceSession = %q, want %q", got.SourceSession, "ses_42")
	}
	if authoredInfo := byName["manual-note"]; authoredInfo.CreatedBy != "" || authoredInfo.SourceSession != "" {
		t.Errorf("authored draft carried provenance: %+v", authoredInfo)
	}
}

func TestListDraftsExcludesPromotedDraft(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	handle, err := store.SaveDraft(t.Context(), skills.Draft{
		Name:        "promoted",
		Description: "A draft that will be promoted out of the review queue.",
		Body:        "step one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(t.Context(), handle); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	drafts, err := store.ListDrafts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 0 {
		t.Fatalf("promoted draft still listed: %+v", drafts)
	}
}

func TestPromoteRevisionReplacesActiveAndArchivesOld(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	installActive(t, store, "run-tests", "old body: use make test")
	promoteRevision(t, store, "run-tests", "new body: use go test ./...")

	active, err := os.ReadFile(filepath.Join(root, "run-tests", "SKILL.md"))
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if !strings.Contains(string(active), "go test ./...") {
		t.Fatalf("active not replaced with the revision:\n%s", active)
	}
	archived, err := os.ReadFile(filepath.Join(root, "_archive", "run-tests", "SKILL.md"))
	if err != nil {
		t.Fatalf("superseded version not archived: %v", err)
	}
	if !strings.Contains(string(archived), "make test") {
		t.Fatalf("archived version is not the superseded one:\n%s", archived)
	}
}

func TestPromoteRevisionOverwritesStaleArchiveSlot(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	installActive(t, store, "note", "body v1")
	promoteRevision(t, store, "note", "body v2") // archives v1
	promoteRevision(t, store, "note", "body v3") // archives v2, overwriting v1

	active, err := os.ReadFile(filepath.Join(root, "note", "SKILL.md"))
	if err != nil || !strings.Contains(string(active), "body v3") {
		t.Fatalf("active = %q, err=%v; want body v3", active, err)
	}
	archived, err := os.ReadFile(filepath.Join(root, "_archive", "note", "SKILL.md"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.Contains(string(archived), "body v2") || strings.Contains(string(archived), "body v1") {
		t.Fatalf("archive should hold only the immediately-superseded v2:\n%s", archived)
	}
}

func TestPromoteNewSkillStillConflictsWithActive(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir())
	installActive(t, store, "dup", "original body")

	// A non-revising draft with the same name but different bytes must NOT
	// overwrite the active skill.
	handle, err := store.SaveDraft(t.Context(), skills.Draft{
		Name:        "dup",
		Description: "A skill with a description long enough to validate.",
		Body:        "colliding body",
	})
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := store.Promote(t.Context(), handle); err == nil {
		t.Fatal("promoting a non-revising same-name draft should conflict, not overwrite")
	}
}

func TestListDraftsDisabledStoreIsEmpty(t *testing.T) {
	store := skillauthoring.NewStore("")
	drafts, err := store.ListDrafts(t.Context())
	if err != nil {
		t.Fatalf("ListDrafts on disabled store: %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("disabled store returned %d drafts", len(drafts))
	}
}
