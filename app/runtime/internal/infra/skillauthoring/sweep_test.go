package skillauthoring_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

const (
	sweepArchive = 30 * 24 * time.Hour
)

var sweepBase = time.Unix(1_700_000_000, 0)

// installActiveAgentSkill approves an agent-authored skill (origin=agent) so the
// provenance-gated curator will consider it.
func installActiveAgentSkill(t *testing.T, store *skillauthoring.Store, name string) {
	t.Helper()
	ref, err := store.SubmitProposal(t.Context(), skills.Proposal{Scope: skills.ScopeUser,
		Name:         name,
		Description:  "An agent-authored skill with a long enough description.",
		Instructions: "instructions",
		Origin:       skills.ProposalOriginMined,
	})
	if err != nil {
		t.Fatalf("SubmitProposal(%s): %v", name, err)
	}
	if err := store.ApproveProposal(t.Context(), ref); err != nil {
		t.Fatalf("ApproveProposal(%s): %v", name, err)
	}
}

func TestSweepIdleArchivesOnlyIdleAgentSkills(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	installActive(t, store, "human-skill", "instructions") // no provenance → human-authored

	// First sweep seeds FirstSeen for both; nothing is idle yet.
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("first sweep archived %v", archived)
	}

	// Far past the archive threshold: the agent skill is idle, the human one is exempt.
	later := sweepBase.Add(sweepArchive + time.Hour)
	archived, err = store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0] != "agent-skill" {
		t.Fatalf("archived = %v, want [agent-skill]", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "_archive", "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("agent-skill not archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "human-skill", "SKILL.md")); err != nil {
		t.Fatalf("human-skill must stay active (provenance gate): %v", err)
	}
}

func TestSweepIdleGivesNeverSweptSkillGrace(t *testing.T) {
	store := skillauthoring.NewStore(t.TempDir(), skills.ScopeUser)
	installActiveAgentSkill(t, store, "fresh")
	// A skill first seen at this sweep gets FirstSeen=now, so it can't be idle yet.
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("archived a skill within its grace floor: %v", archived)
	}
}

func TestSweepIdleRestoredSkillGetsFreshGrace(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	if _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive); err != nil {
		t.Fatal(err)
	}
	later := sweepBase.Add(sweepArchive + time.Hour)
	if _, err := store.SweepIdle(t.Context(), later, sweepArchive); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// An immediate re-sweep at the same instant must NOT re-archive the just-restored
	// skill: archiving dropped its usage record, so it starts a fresh grace floor.
	archived, err := store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("re-archived a just-restored skill: %v", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("restored skill should be active: %v", err)
	}
}

func TestManualArchiveThenRestoreGetsFreshGrace(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
	installActiveAgentSkill(t, store, "agent-skill")
	// Seed a usage record with an old activity time.
	if _, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive); err != nil {
		t.Fatal(err)
	}
	// A human archives then restores it much later. Archiving drops the usage
	// record, so the restored skill starts a fresh grace floor.
	if err := store.Archive(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	later := sweepBase.Add(sweepArchive + time.Hour)
	archived, err := store.SweepIdle(t.Context(), later, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("re-archived a manually archived-then-restored skill: %v", archived)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-skill", "SKILL.md")); err != nil {
		t.Fatalf("restored skill should be active: %v", err)
	}
}

func TestSweepIdleDisabledStoreNoOps(t *testing.T) {
	store := skillauthoring.NewStore("", skills.ScopeUser)
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepArchive)
	if err != nil || archived != nil {
		t.Fatalf("disabled sweep = %v, %v", archived, err)
	}
}
