package sqlite_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newAgentMemoryStore(t *testing.T) *sqlite.AgentMemoryStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewAgentMemoryStore(db)
}

func appendAgentFacts(t *testing.T, store *sqlite.AgentMemoryStore, project, day string, facts ...string) []knowledge.LedgerFact {
	t.Helper()
	inserted, err := store.AppendLedger(t.Context(), knowledge.FactBatch{
		Project: project, SessionID: "ses_1", Day: day, Facts: facts,
		CapturedAt: time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return inserted
}

func TestAgentMemoryLedgerIsDailyDeduplicatedAndProjectScoped(t *testing.T) {
	store := newAgentMemoryStore(t)
	first := appendAgentFacts(t, store, "/repo/a", "2026-07-18", "- one", "- two")
	second := appendAgentFacts(t, store, "/repo/a", "2026-07-19", "two", "three")
	other := appendAgentFacts(t, store, "/repo/b", "2026-07-19", "one")
	if len(first) != 2 || len(second) != 1 || len(other) != 1 {
		t.Fatalf("insert counts = %d, %d, %d; want 2, 1, 1", len(first), len(second), len(other))
	}

	pending, err := store.PendingLedger(t.Context(), "/repo/a", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 || pending[0].Day != "2026-07-18" || pending[2].Day != "2026-07-19" {
		t.Fatalf("pending = %+v", pending)
	}
	otherPending, err := store.PendingLedger(t.Context(), "/repo/b", 0, 10)
	if err != nil || len(otherPending) != 1 || otherPending[0].Content != "- one" {
		t.Fatalf("other project pending = (%+v, %v)", otherPending, err)
	}
}

func TestAgentMemoryPublishAtomicallyAdvancesWatermark(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	through := facts[len(facts)-1].Sequence
	updatedAt := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	published, err := store.PublishCuratedMemory(t.Context(), "/repo", 0, through, "# MEMORY\n\n- one", updatedAt)
	if err != nil || !published {
		t.Fatalf("PublishCuratedMemory = (%v, %v)", published, err)
	}
	memory, err := store.CuratedMemory(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if memory.Content != "# MEMORY\n\n- one" || memory.Watermark != through || !memory.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("curated memory = %+v", memory)
	}
	stale, err := store.PublishCuratedMemory(t.Context(), "/repo", 0, through, "stale", updatedAt.Add(time.Hour))
	if err != nil || stale {
		t.Fatalf("stale publish = (%v, %v), want false, nil", stale, err)
	}
	memory, _ = store.CuratedMemory(t.Context(), "/repo")
	if memory.Content != "# MEMORY\n\n- one" || memory.Watermark != through {
		t.Fatalf("stale publish changed memory: %+v", memory)
	}
	if pending, err := store.PendingLedger(t.Context(), "/repo", memory.Watermark, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending after publish = (%+v, %v)", pending, err)
	}
}

func TestAgentMemoryPublishCASHasOneWinner(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one")
	through := facts[0].Sequence
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			published, err := store.PublishCuratedMemory(t.Context(), "/repo", 0, through, "body", time.Now())
			if err != nil {
				t.Errorf("publish: %v", err)
				return
			}
			if published {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("publish winners = %d, want 1", got)
	}
}

func TestAgentMemoryConcurrentAppendDeduplicates(t *testing.T) {
	store := newAgentMemoryStore(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.AppendLedger(t.Context(), knowledge.FactBatch{
				Project: "/repo", SessionID: "ses_1", Day: "2026-07-19",
				Facts: []string{"same fact"}, CapturedAt: time.Now(),
			})
			if err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	wg.Wait()
	pending, err := store.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("concurrent pending = (%+v, %v), want one fact", pending, err)
	}
}
