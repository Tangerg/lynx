package skillauthoring_test

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

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
