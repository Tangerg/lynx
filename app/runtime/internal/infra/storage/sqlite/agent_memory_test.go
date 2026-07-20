package sqlite_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
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

func appendAgentFacts(t *testing.T, store *sqlite.AgentMemoryStore, project, day string, facts ...string) []agentmemory.LedgerFact {
	t.Helper()
	inserted, err := store.AppendLedger(t.Context(), agentmemory.FactBatch{
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

func TestAgentMemoryReconcileAdvancesWatermarkAndItems(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	through := facts[len(facts)-1].Sequence
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	published, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"- one", "- two"}, now)
	if err != nil || !published {
		t.Fatalf("Reconcile = (%v, %v)", published, err)
	}
	state, err := store.State(t.Context(), "/repo")
	if err != nil || state.Watermark != through || !state.UpdatedAt.Equal(now) {
		t.Fatalf("state = %+v, err=%v", state, err)
	}
	items, err := store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != 2 {
		t.Fatalf("items = (%+v, %v)", items, err)
	}
	for _, item := range items {
		if item.Origin != agentmemory.OriginAuto || item.Scope != agentmemory.ScopeProject {
			t.Fatalf("item provenance = %+v", item)
		}
	}

	// A second reconcile that expects watermark 0 again has lost the CAS: it must
	// neither advance the watermark nor rewrite the item set.
	stale, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"- three"}, now.Add(time.Hour))
	if err != nil || stale {
		t.Fatalf("stale reconcile = (%v, %v), want false, nil", stale, err)
	}
	items, _ = store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
	if len(items) != 2 {
		t.Fatalf("stale reconcile changed items: %+v", items)
	}
	if pending, err := store.PendingLedger(t.Context(), "/repo", state.Watermark, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending after reconcile = (%+v, %v)", pending, err)
	}
}

func TestAgentMemoryReconcilePreservesUnchangedAndPrunesRemoved(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two", "three")
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(t.Context(), "/repo", 0, facts[1].Sequence, []string{"- one", "- two"}, now); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
	idByContent := make(map[string]string, len(before))
	for _, item := range before {
		idByContent[item.Content] = item.ID
	}

	// Drop "- two", keep "- one", add "- three".
	if _, err := store.Reconcile(t.Context(), "/repo", facts[1].Sequence, facts[2].Sequence, []string{"- one", "- three"}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
	got := make(map[string]string, len(after))
	for _, item := range after {
		got[item.Content] = item.ID
	}
	if len(after) != 2 || got["- two"] != "" {
		t.Fatalf("prune failed: %+v", after)
	}
	if got["- one"] == "" || got["- one"] != idByContent["- one"] {
		t.Fatalf("unchanged item lost its stable id: %q -> %q", idByContent["- one"], got["- one"])
	}
	if got["- three"] == "" {
		t.Fatal("new item was not inserted")
	}
}

func TestAgentMemoryEmbeddingBackfillRoundTrip(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(t.Context(), "/repo", 0, facts[1].Sequence, []string{"- one", "- two"}, now); err != nil {
		t.Fatal(err)
	}

	// Fresh items carry no embedding.
	unembedded, err := store.UnembeddedItems(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(unembedded) != 2 {
		t.Fatalf("unembedded = (%+v, %v), want 2", unembedded, err)
	}
	vectors := make(map[string][]float32, len(unembedded))
	for i, item := range unembedded {
		vectors[item.ID] = []float32{float32(i + 1), 0.5}
	}
	if err := store.SetEmbeddings(t.Context(), vectors); err != nil {
		t.Fatal(err)
	}

	// After backfill nothing is unembedded, and the search fetch decodes vectors.
	if rest, err := store.UnembeddedItems(t.Context(), agentmemory.ScopeProject, "/repo"); err != nil || len(rest) != 0 {
		t.Fatalf("unembedded after backfill = (%+v, %v), want 0", rest, err)
	}
	forSearch, err := store.ItemsForSearch(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(forSearch) != 2 {
		t.Fatalf("items for search = (%+v, %v)", forSearch, err)
	}
	for _, item := range forSearch {
		want := vectors[item.ID]
		if len(item.Embedding) != len(want) || item.Embedding[0] != want[0] || item.Embedding[1] != want[1] {
			t.Fatalf("embedding round-trip failed for %s: got %v want %v", item.ID, item.Embedding, want)
		}
	}
}

func TestAgentMemoryReconcileCASHasOneWinner(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one")
	through := facts[0].Sequence
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			published, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"- body"}, time.Now())
			if err != nil {
				t.Errorf("reconcile: %v", err)
				return
			}
			if published {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("reconcile winners = %d, want 1", got)
	}
}

func TestAgentMemoryConcurrentAppendDeduplicates(t *testing.T) {
	store := newAgentMemoryStore(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := store.AppendLedger(t.Context(), agentmemory.FactBatch{
				Project: "/repo", SessionID: "ses_1", Day: "2026-07-19",
				Facts: []string{"same fact"}, CapturedAt: time.Now(),
			})
			if err != nil {
				t.Errorf("append: %v", err)
			}
		})
	}
	wg.Wait()
	pending, err := store.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("concurrent pending = (%+v, %v), want one fact", pending, err)
	}
}
