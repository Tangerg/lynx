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
	sweepStale   = 7 * 24 * time.Hour
	sweepArchive = 30 * 24 * time.Hour
)

var sweepBase = time.Unix(1_700_000_000, 0)

// installAgentActive promotes an agent-authored skill (created_by=agent) so the
// provenance-gated curator will consider it.
func installAgentActive(t *testing.T, store *skillauthoring.Store, name, body string) {
	t.Helper()
	handle, err := store.SaveDraft(t.Context(), skills.Draft{
		Name:        name,
		Description: "An agent-authored skill with a long enough description.",
		Body:        body,
		CreatedBy:   skills.CreatedByAgent,
	})
	if err != nil {
		t.Fatalf("SaveDraft(%s): %v", name, err)
	}
	if err := store.Promote(t.Context(), handle); err != nil {
		t.Fatalf("Promote(%s): %v", name, err)
	}
}

func TestSweepIdleArchivesOnlyIdleAgentSkills(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	installAgentActive(t, store, "agent-skill", "body")
	installActive(t, store, "human-skill", "body") // no provenance → human-authored

	// First sweep seeds FirstSeen for both; nothing is idle yet.
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepStale, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("first sweep archived %v", archived)
	}

	// Far past the archive threshold: the agent skill is idle, the human one is exempt.
	later := sweepBase.Add(sweepArchive + time.Hour)
	archived, err = store.SweepIdle(t.Context(), later, sweepStale, sweepArchive)
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
	store := skillauthoring.NewStore(t.TempDir())
	installAgentActive(t, store, "fresh", "body")
	// A skill first seen at this sweep gets FirstSeen=now, so it can't be idle yet.
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepStale, sweepArchive)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("archived a skill within its grace floor: %v", archived)
	}
}

func TestSweepIdleRestoredSkillGetsFreshGrace(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	installAgentActive(t, store, "agent-skill", "body")
	if _, err := store.SweepIdle(t.Context(), sweepBase, sweepStale, sweepArchive); err != nil {
		t.Fatal(err)
	}
	later := sweepBase.Add(sweepArchive + time.Hour)
	if _, err := store.SweepIdle(t.Context(), later, sweepStale, sweepArchive); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(t.Context(), "agent-skill"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// An immediate re-sweep at the same instant must NOT re-archive the just-restored
	// skill: archiving dropped its usage record, so it starts a fresh grace floor.
	archived, err := store.SweepIdle(t.Context(), later, sweepStale, sweepArchive)
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

func TestSweepIdleDisabledStoreNoOps(t *testing.T) {
	store := skillauthoring.NewStore("")
	archived, err := store.SweepIdle(t.Context(), sweepBase, sweepStale, sweepArchive)
	if err != nil || archived != nil {
		t.Fatalf("disabled sweep = %v, %v", archived, err)
	}
}
