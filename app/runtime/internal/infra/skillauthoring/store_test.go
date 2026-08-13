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
	proposalSubdir = "_proposals"
	archiveSubdir  = "_archive"
)

func TestSubmitProposalThenApproveProposal(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	proposal := skills.Proposal{Scope: skills.ScopeUser,
		Name:         "git-bisect-helper",
		Description:  "Walk a git bisect to find a regression; use it when a test started failing.",
		Instructions: "# Steps\n1. `git bisect start`\n2. mark good/bad\n",
	}

	ref, submitted, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatalf("SubmitProposal: %v", err)
	}
	if len(submitted) != 1 || submitted[0] != filepath.Join(root, "_proposals", ref.Revision, skillspec.SkillFile) {
		t.Fatalf("SubmitProposal identities = %v", submitted)
	}
	if duplicate, replayed, err := store.SubmitProposal(t.Context(), proposal); err != nil || duplicate != ref || len(replayed) != 0 {
		t.Fatalf("replayed SubmitProposal = (%+v, %v, %v), want original ref and no mutation", duplicate, replayed, err)
	}
	// The proposal is under _proposals (invisible to the read-only source) — not active.
	if _, err := os.Stat(filepath.Join(root, "_proposals", ref.Revision, "SKILL.md")); err != nil {
		t.Fatalf("proposal not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, proposal.Name, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("an unapproved proposal must not appear in the active set")
	}

	approved, err := store.ApproveProposal(t.Context(), ref)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if len(approved) != 2 {
		t.Fatalf("ApproveProposal identities = %v, want proposal and active files", approved)
	}
	// Now active, and the proposal is gone.
	if _, err := os.Stat(filepath.Join(root, "_proposals", ref.Revision)); !os.IsNotExist(err) {
		t.Fatal("approval should remove the proposal")
	}

	// The approved skill is discoverable + valid per the read-only spec loader.
	source := skillspec.Dir(root)
	summaries, err := source.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Name != proposal.Name {
		t.Fatalf("approved skill not discoverable: %+v", summaries)
	}
	loaded, err := source.Load(t.Context(), proposal.Name)
	if err != nil {
		t.Fatalf("Load approved skill: %v", err)
	}
	if loaded.Description != proposal.Description {
		t.Fatalf("loaded description = %q, want %q", loaded.Description, proposal.Description)
	}
}

func approve(t *testing.T, store *skillauthoring.Store, name string) {
	t.Helper()
	d := skills.Proposal{Scope: skills.ScopeUser, Name: name, Description: "A description that is long enough to validate.", Instructions: "do the thing"}
	ref, _, err := store.SubmitProposal(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); err != nil {
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
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	approve(t, store, "alpha-skill")
	approve(t, store, "beta-skill")

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
	archived, err := store.Archive(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(archived) != 2 {
		t.Fatalf("Archive identities = %v, want active and archive files", archived)
	}
	replayedArchive, err := store.Archive(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("replayed Archive: %v", err)
	}
	if len(replayedArchive) != 0 {
		t.Fatalf("replayed Archive identities = %v, want none", replayedArchive)
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
	restored, err := store.Restore(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 2 {
		t.Fatalf("Restore identities = %v, want archive and active files", restored)
	}
	replayedRestore, err := store.Restore(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("replayed Restore: %v", err)
	}
	if len(replayedRestore) != 0 {
		t.Fatalf("replayed Restore identities = %v, want none", replayedRestore)
	}
	list, _ = store.List(t.Context())
	if lc, _ := lifecycleOf(list, "alpha-skill"); lc != skills.Active {
		t.Fatalf("restored alpha should be active, got %q", lc)
	}
}

func TestArchiveMissingErrors(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	if _, err := store.Archive(t.Context(), "nope"); err == nil {
		t.Fatal("archiving a nonexistent skill must error")
	}
	if _, err := store.Restore(t.Context(), "nope"); err == nil {
		t.Fatal("restoring a nonexistent archived skill must error")
	}
}

func TestApproveProposalMissingProposalErrors(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	ref := skills.NewProposalRef(skills.ScopeUser, "never-proposed", []byte("missing"))
	if _, err := store.ApproveProposal(t.Context(), ref); err == nil {
		t.Fatal("promoting a nonexistent proposal must error")
	}
}

func TestRejectProposal(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "throwaway", Description: "A description long enough to pass validation.", Instructions: "instructions"}
	ref, _, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RejectProposal(t.Context(), ref); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_proposals", ref.Revision)); !os.IsNotExist(err) {
		t.Fatal("reject should remove the proposal dir")
	}
}

func TestSubmitProposalRejectsInvalid(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	// Invalid name (uppercase / spaces) is refused before anything is written.
	if _, _, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser, Name: "Bad Name", Description: "desc that is long enough", Instructions: "b"}); err == nil {
		t.Fatal("invalid skill name must be rejected")
	}
}

func TestSameNameProposalsKeepIndependentApprovedBytes(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	first := skills.Proposal{Scope: skills.ScopeUser, Name: "shared-name", Description: "The first independently approved skill version.", Instructions: "first instructions"}
	second := skills.Proposal{Scope: skills.ScopeUser, Name: "shared-name", Description: "The second independently approved skill version.", Instructions: "second instructions"}

	firstRef, _, err := store.SubmitProposal(t.Context(), first)
	if err != nil {
		t.Fatalf("save first proposal: %v", err)
	}
	secondRef, _, err := store.SubmitProposal(t.Context(), second)
	if err != nil {
		t.Fatalf("save second proposal: %v", err)
	}
	if firstRef == secondRef {
		t.Fatal("different proposal bytes received the same ref")
	}
	if _, err := store.RejectProposal(t.Context(), firstRef); err != nil {
		t.Fatalf("reject first proposal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, proposalSubdir, secondRef.Revision, "SKILL.md")); err != nil {
		t.Fatalf("rejecting one proposal removed another: %v", err)
	}
	if _, err := store.ApproveProposal(t.Context(), secondRef); err != nil {
		t.Fatalf("approve second proposal: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, second.Name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read active skill: %v", err)
	}
	if !secondRef.Matches(content) {
		t.Fatal("approved bytes do not match the approved ref")
	}
}

func TestApproveProposalRejectsChangedProposalWithoutTouchingActiveSet(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "immutable-proposal", Description: "Verify immutable proposal publication semantics.", Instructions: "approved instructions"}
	ref, _, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatalf("SubmitProposal: %v", err)
	}
	path := filepath.Join(root, proposalSubdir, ref.Revision, "SKILL.md")
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper proposal: %v", err)
	}

	if _, err := store.ApproveProposal(t.Context(), ref); !errors.Is(err, skills.ErrProposalChanged) {
		t.Fatalf("ApproveProposal() error = %v, want ErrProposalChanged", err)
	}
	if _, err := os.Stat(filepath.Join(root, proposal.Name)); !os.IsNotExist(err) {
		t.Fatal("a changed proposal reached the active set")
	}
	if _, err := store.RejectProposal(t.Context(), ref); !errors.Is(err, skills.ErrProposalChanged) {
		t.Fatalf("RejectProposal() error = %v, want ErrProposalChanged", err)
	}
}

func TestApproveProposalIsIdempotentForExactReplay(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "replay-safe", Description: "Make suspended proposal replay deterministic.", Instructions: "same instructions"}
	first, _, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatalf("first SubmitProposal: %v", err)
	}
	second, _, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatalf("second SubmitProposal: %v", err)
	}
	if first != second {
		t.Fatalf("replayed refs differ: %+v vs %+v", first, second)
	}
	if _, err := store.ApproveProposal(t.Context(), first); err != nil {
		t.Fatalf("first ApproveProposal: %v", err)
	}
	if _, _, err := store.SubmitProposal(t.Context(), proposal); err != nil {
		t.Fatalf("restage replay: %v", err)
	}
	if _, err := store.ApproveProposal(t.Context(), first); err != nil {
		t.Fatalf("replayed ApproveProposal: %v", err)
	}
}

func TestLifecycleConflictsPreserveBothStates(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	approve(t, store, "conflict-safe")
	if _, err := store.Archive(t.Context(), "conflict-safe"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "conflict-safe", Description: "A different version must not replace the archive.", Instructions: "replacement"}
	ref, _, err := store.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatalf("SubmitProposal: %v", err)
	}
	if _, err := store.ApproveProposal(t.Context(), ref); !errors.Is(err, skills.ErrConflict) {
		t.Fatalf("ApproveProposal() error = %v, want ErrConflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, archiveSubdir, proposal.Name, "SKILL.md")); err != nil {
		t.Fatalf("archived version was lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, proposalSubdir, ref.Revision, "SKILL.md")); err != nil {
		t.Fatalf("conflicting proposal was lost: %v", err)
	}
}

func TestConcurrentApprovalsPublishOneRevisionWithoutLosingTheOther(t *testing.T) {
	root := t.TempDir()
	stores := []*skillauthoring.Store{skillauthoring.NewStore(root, skills.ScopeUser), skillauthoring.NewStore(root, skills.ScopeUser)}
	proposals := []skills.Proposal{
		{Scope: skills.ScopeUser, Name: "ordered-publish", Description: "The first concurrently proposed skill revision.", Instructions: "first"},
		{Scope: skills.ScopeUser, Name: "ordered-publish", Description: "The second concurrently proposed skill revision.", Instructions: "second"},
	}
	refs := make([]skills.ProposalRef, len(proposals))
	for i, proposal := range proposals {
		ref, _, err := stores[i].SubmitProposal(t.Context(), proposal)
		if err != nil {
			t.Fatalf("SubmitProposal(%d): %v", i, err)
		}
		refs[i] = ref
	}

	errs := make([]error, len(refs))
	var wait sync.WaitGroup
	for i, ref := range refs {
		wait.Go(func() { _, errs[i] = stores[i].ApproveProposal(t.Context(), ref) })
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
			t.Fatalf("unexpected approval error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("approval outcomes = %d success, %d conflict; want 1 and 1", succeeded, conflicted)
	}

	active, err := os.ReadFile(filepath.Join(root, proposals[0].Name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read active skill: %v", err)
	}
	winner := -1
	for i, ref := range refs {
		if ref.Matches(active) {
			winner = i
			break
		}
	}
	if winner < 0 {
		t.Fatal("active bytes match neither proposed revision")
	}
	loser := 1 - winner
	if _, err := os.Stat(filepath.Join(root, proposalSubdir, refs[loser].Revision, "SKILL.md")); err != nil {
		t.Fatalf("losing revision was destroyed: %v", err)
	}
}
