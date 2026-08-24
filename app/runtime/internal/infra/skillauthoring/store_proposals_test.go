package skillauthoring_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// installActive approves a plain (non-revising) proposal so a named skill is active.
func installActive(t *testing.T, store *skillauthoring.Store, name, instructions string) {
	t.Helper()
	ref, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         name,
		Description:  "A skill with a description long enough to validate.",
		Instructions: instructions,
	})
	if err != nil {
		t.Fatalf("SubmitProposal(%s): %v", name, err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err != nil {
		t.Fatalf("ApproveProposal(%s): %v", name, err)
	}
}

// approveRevision saves + approves a revising proposal for name with a new instructions.
func approveRevision(t *testing.T, store *skillauthoring.Store, name, instructions string) {
	t.Helper()
	ref, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         name,
		Description:  "A skill with a description long enough to validate.",
		Instructions: instructions,
		Origin:       skills.ProposalOriginMined,
		Revises:      true,
	})
	if err != nil {
		t.Fatalf("SubmitProposal(revision %s): %v", name, err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err != nil {
		t.Fatalf("ApproveProposal(revision %s): %v", name, err)
	}
}

func TestListProposalsReportsRefsAndProvenance(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)

	mined := skills.Proposal{Scope: skills.ScopeUser,
		Name:          "run-project-tests",
		Description:   "Run the module test suite. Use when asked to run or verify tests.",
		Instructions:  "Run `go test ./...` from the module root.",
		Origin:        skills.ProposalOriginMined,
		SourceSession: "ses_42",
	}
	authored := skills.Proposal{Scope: skills.ScopeUser,
		Name:         "manual-note",
		Description:  "A proposal with no provenance, as a human proposal would carry.",
		Instructions: "do the thing",
	}
	minedRef, _, err := store.SubmitProposal(t.Context(), mined)
	if err != nil {
		t.Fatalf("SubmitProposal(mined): %v", err)
	}
	if _, _, err := store.SubmitProposal(t.Context(), authored); err != nil {
		t.Fatalf("SubmitProposal(authored): %v", err)
	}

	proposals, err := store.ListProposals(t.Context())
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("ListProposals returned %d proposals, want 2", len(proposals))
	}

	byName := map[string]skills.ProposalReview{}
	for _, d := range proposals {
		byName[d.Ref.Name] = d
	}
	got := byName["run-project-tests"]
	if got.Ref != minedRef {
		t.Errorf("mined ref = %+v, want %+v", got.Ref, minedRef)
	}
	if got.Description != mined.Description {
		t.Errorf("mined description = %q", got.Description)
	}
	if got.Origin != skills.ProposalOriginMined {
		t.Errorf("mined Origin = %q, want %q", got.Origin, skills.ProposalOriginMined)
	}
	if got.SourceSession != "ses_42" {
		t.Errorf("mined SourceSession = %q, want %q", got.SourceSession, "ses_42")
	}
	if authoredInfo := byName["manual-note"]; authoredInfo.Origin != "" || authoredInfo.SourceSession != "" {
		t.Errorf("authored proposal carried provenance: %+v", authoredInfo)
	}
}

func TestListProposalsExcludesApprovedProposal(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	ref, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         "approved",
		Description:  "A proposal that will be approved out of the review queue.",
		Instructions: "step one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	proposals, err := store.ListProposals(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 0 {
		t.Fatalf("approved proposal still listed: %+v", proposals)
	}
}

func TestSubmitProposalSupersedesPendingProposalWithSameName(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	first := skills.Proposal{
		Scope: skills.ScopeUser, Name: "current-review",
		Description:  "The first version of one proposal awaiting review.",
		Instructions: "first instructions",
	}
	second := first
	second.Description = "The current version of one proposal awaiting review."
	second.Instructions = "second instructions"

	firstRef, _, err := store.SubmitProposal(t.Context(), first)
	if err != nil {
		t.Fatalf("SubmitProposal(first): %v", err)
	}
	secondRef, _, err := store.SubmitProposal(t.Context(), second)
	if err != nil {
		t.Fatalf("SubmitProposal(second): %v", err)
	}
	proposals, err := store.ListProposals(t.Context())
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(proposals) != 1 || proposals[0].Ref != secondRef {
		t.Fatalf("pending proposals = %+v, want only current revision %+v", proposals, secondRef)
	}
	if _, err := store.ApproveProposal(t.Context(), firstRef); err == nil {
		t.Fatal("superseded proposal revision remained reviewable")
	}
}

func TestSubmitProposalBoundsDocumentAndPendingQueue(t *testing.T) {
	t.Run("document", func(t *testing.T) {
		store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
		oversized := skills.Proposal{
			Scope: skills.ScopeUser, Name: "oversized-proposal",
			Description:  "A proposal whose rendered document exceeds the authored resource envelope.",
			Instructions: strings.Repeat("x", (1<<20)+1),
		}
		if _, _, err := store.SubmitProposal(t.Context(), oversized); !errors.Is(err, skills.ErrDocumentTooLarge) {
			t.Fatalf("SubmitProposal error = %v, want ErrDocumentTooLarge", err)
		}
	})

	t.Run("queue", func(t *testing.T) {
		store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
		for i := range 129 {
			proposal := skills.Proposal{
				Scope: skills.ScopeUser, Name: fmt.Sprintf("bounded-proposal-%03d", i),
				Description:  "One proposal in the bounded human review queue.",
				Instructions: "review these instructions",
			}
			_, _, err := store.SubmitProposal(t.Context(), proposal)
			if i < 128 && err != nil {
				t.Fatalf("SubmitProposal(%d): %v", i, err)
			}
			if i == 128 && !errors.Is(err, skills.ErrProposalQueueFull) {
				t.Fatalf("SubmitProposal(128) error = %v, want ErrProposalQueueFull", err)
			}
		}
	})
}

func TestListProposalsRejectsCorruptUnboundedStorage(t *testing.T) {
	t.Run("oversized document", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "_proposals", "oversized-on-disk")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(dir, "SKILL.md"),
			[]byte(strings.Repeat("x", skills.MaxAuthoredSkillDocumentBytes+1)),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		store := skillauthoring.NewStore(root, skills.ScopeUser)
		if _, err := store.ListProposals(t.Context()); !errors.Is(err, skills.ErrDocumentTooLarge) {
			t.Fatalf("ListProposals error = %v, want ErrDocumentTooLarge", err)
		}
	})

	t.Run("over-capacity queue", func(t *testing.T) {
		root := t.TempDir()
		for i := range skills.MaxPendingProposalsPerScope + 1 {
			if err := os.MkdirAll(
				filepath.Join(root, "_proposals", fmt.Sprintf("external-proposal-%03d", i)),
				0o755,
			); err != nil {
				t.Fatal(err)
			}
		}
		store := skillauthoring.NewStore(root, skills.ScopeUser)
		if _, err := store.ListProposals(t.Context()); !errors.Is(err, skills.ErrProposalQueueFull) {
			t.Fatalf("ListProposals error = %v, want ErrProposalQueueFull", err)
		}
	})
}

func TestConcurrentStoresAdmitExactlyOneRemainingQueueSlot(t *testing.T) {
	root := t.TempDir()
	seed := skillauthoring.NewStore(root, skills.ScopeUser)
	for i := range skills.MaxPendingProposalsPerScope - 1 {
		_, _, err := seed.SubmitProposal(t.Context(), skills.Proposal{
			Scope: skills.ScopeUser, Name: fmt.Sprintf("seed-proposal-%03d", i),
			Description:  "A proposal occupying one bounded review queue slot.",
			Instructions: "review these instructions",
		})
		if err != nil {
			t.Fatalf("seed proposal %d: %v", i, err)
		}
	}
	stores := []*skillauthoring.Store{
		skillauthoring.NewStore(root, skills.ScopeUser),
		skillauthoring.NewStore(root, skills.ScopeUser),
	}
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := range stores {
		wait.Go(func() {
			<-start
			_, _, errs[i] = stores[i].SubmitProposal(t.Context(), skills.Proposal{
				Scope: skills.ScopeUser, Name: fmt.Sprintf("concurrent-proposal-%d", i),
				Description:  "A concurrent proposal competing for the final queue slot.",
				Instructions: "review these instructions",
			})
		})
	}
	close(start)
	wait.Wait()
	succeeded, full := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, skills.ErrProposalQueueFull):
			full++
		default:
			t.Fatalf("concurrent submission error = %v", err)
		}
	}
	if succeeded != 1 || full != 1 {
		t.Fatalf("concurrent outcomes = %d success / %d full, want 1 / 1", succeeded, full)
	}
	pending, err := seed.ListProposals(t.Context())
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(pending) != skills.MaxPendingProposalsPerScope {
		t.Fatalf("pending count = %d, want %d", len(pending), skills.MaxPendingProposalsPerScope)
	}
}

func TestApproveProposalRevisionReplacesActiveAndArchivesOld(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	installActive(t, store, "run-tests", "old instructions: use make test")
	approveRevision(t, store, "run-tests", "new instructions: use go test ./...")

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

func TestApproveProposalRevisionOverwritesStaleArchiveSlot(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	installActive(t, store, "note", "instructions v1")
	approveRevision(t, store, "note", "instructions v2") // archives v1
	approveRevision(t, store, "note", "instructions v3") // archives v2, overwriting v1

	active, err := os.ReadFile(filepath.Join(root, "note", "SKILL.md"))
	if err != nil || !strings.Contains(string(active), "instructions v3") {
		t.Fatalf("active = %q, err=%v; want instructions v3", active, err)
	}
	archived, err := os.ReadFile(filepath.Join(root, "_archive", "note", "SKILL.md"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.Contains(string(archived), "instructions v2") || strings.Contains(string(archived), "instructions v1") {
		t.Fatalf("archive should hold only the immediately-superseded v2:\n%s", archived)
	}
}

func TestApproveProposalNewSkillStillConflictsWithActive(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	installActive(t, store, "dup", "original instructions")

	// A non-revising proposal with the same name but different bytes must NOT
	// overwrite the active skill.
	ref, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         "dup",
		Description:  "A skill with a description long enough to validate.",
		Instructions: "colliding instructions",
	})
	if err != nil {
		t.Fatalf("SubmitProposal: %v", err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err == nil {
		t.Fatal("approving a non-revising same-name proposal should conflict, not overwrite")
	}
}

func TestListProposalsDisabledStoreIsEmpty(t *testing.T) {
	store := skillauthoring.NewStore("", skills.ScopeUser)
	proposals, err := store.ListProposals(t.Context())
	if err != nil {
		t.Fatalf("ListProposals on disabled store: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("disabled store returned %d proposals", len(proposals))
	}
}
