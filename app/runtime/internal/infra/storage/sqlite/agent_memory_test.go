package sqlite_test

import (
	"errors"
	"path/filepath"
	"slices"
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
	first := appendAgentFacts(t, store, "/repo/a", "2026-07-18", "one", "two")
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
	if err != nil || len(otherPending) != 1 || otherPending[0].Content != "one" {
		t.Fatalf("other project pending = (%+v, %v)", otherPending, err)
	}
}

func TestAgentMemoryReconcileAdvancesWatermarkAndItems(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	through := facts[len(facts)-1].Sequence
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	published, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"one", "two"}, now)
	if err != nil || !published {
		t.Fatalf("Reconcile = (%v, %v)", published, err)
	}
	state, err := store.State(t.Context(), "/repo")
	if err != nil || state.Watermark != through || !state.UpdatedAt.Equal(now) {
		t.Fatalf("state = %+v, err=%v", state, err)
	}
	// Curated facts land as PENDING proposals — not injected until approved.
	if active, err := store.Items(t.Context(), agentmemory.ScopeProject, "/repo"); err != nil || len(active) != 0 {
		t.Fatalf("active items before approval = (%+v, %v), want none", active, err)
	}
	listed, err := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed = (%+v, %v)", listed, err)
	}
	for _, item := range listed {
		if item.Status != agentmemory.StatusPending || item.Origin != agentmemory.OriginAuto {
			t.Fatalf("proposal = %+v, want pending/auto", item)
		}
	}

	// A second reconcile that expects watermark 0 again has lost the CAS: it must
	// neither advance the watermark nor rewrite the item set.
	stale, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"three"}, now.Add(time.Hour))
	if err != nil || stale {
		t.Fatalf("stale reconcile = (%v, %v), want false, nil", stale, err)
	}
	if listed, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo"); len(listed) != 2 {
		t.Fatalf("stale reconcile changed items: %+v", listed)
	}
	if pending, err := store.PendingLedger(t.Context(), "/repo", state.Watermark, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending after reconcile = (%+v, %v)", pending, err)
	}
}

func TestAgentMemoryReconcilePreservesUnchangedAndPrunesRemoved(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two", "three")
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(t.Context(), "/repo", 0, facts[1].Sequence, []string{"one", "two"}, now); err != nil {
		t.Fatal(err)
	}
	before, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	idByContent := make(map[string]string, len(before))
	for _, item := range before {
		idByContent[item.Content] = item.ID
	}

	// Drop "two", keep "one", add "three" — all still pending proposals.
	if _, err := store.Reconcile(t.Context(), "/repo", facts[1].Sequence, facts[2].Sequence, []string{"one", "three"}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	got := make(map[string]string, len(after))
	for _, item := range after {
		got[item.Content] = item.ID
	}
	if len(after) != 2 || got["two"] != "" {
		t.Fatalf("prune failed: %+v", after)
	}
	if got["one"] == "" || got["one"] != idByContent["one"] {
		t.Fatalf("unchanged item lost its stable id: %q -> %q", idByContent["one"], got["one"])
	}
	if got["three"] == "" {
		t.Fatal("new item was not inserted")
	}
}

func TestAgentMemoryReviewLifecycle(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(t.Context(), "/repo", 0, facts[1].Sequence, []string{"one", "two"}, now); err != nil {
		t.Fatal(err)
	}
	proposals, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if len(proposals) != 2 {
		t.Fatalf("proposals = %d, want 2", len(proposals))
	}
	approve, reject := proposals[0], proposals[1]
	if err := store.SetStatus(t.Context(), approve.ID, agentmemory.StatusActive, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(t.Context(), reject.ID, agentmemory.StatusRejected, now); err != nil {
		t.Fatal(err)
	}

	// Only the approved item is injected; List hides the rejected tombstone.
	active, _ := store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
	if len(active) != 1 || active[0].ID != approve.ID {
		t.Fatalf("active = %+v, want just the approved item", active)
	}
	if listed, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo"); len(listed) != 1 {
		t.Fatalf("list should hide the rejected tombstone: %+v", listed)
	}

	// A later fold re-proposing the rejected fact must NOT resurrect it.
	appendAgentFacts(t, store, "/repo", "2026-07-20", "three")
	pending, _ := store.PendingLedger(t.Context(), "/repo", facts[1].Sequence, 10)
	if _, err := store.Reconcile(t.Context(), "/repo", facts[1].Sequence, pending[len(pending)-1].Sequence,
		[]string{"one", "two", "three"}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	listed, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	var contents []string
	for _, item := range listed {
		contents = append(contents, item.Content)
	}
	if slices.Contains(contents, "two") {
		t.Fatalf("rejected fact was re-proposed: %+v", listed)
	}
}

func TestAgentMemoryManagementOps(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)

	item, err := store.Add(t.Context(), agentmemory.ScopeProject, "/repo", "always run make lint", now)
	if err != nil || item.ID == "" || item.Origin != agentmemory.OriginUser || item.Status != agentmemory.StatusActive {
		t.Fatalf("add = (%+v, %v)", item, err)
	}
	if err := store.SetEmbeddings(t.Context(), map[string][]float32{item.ID: {1, 2, 3}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPinned(t.Context(), item.ID, true, now); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateContent(t.Context(), item.ID, "always run make lint before commit", now); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(t.Context(), item.ID)
	if err != nil || !ok || !got.Pinned || got.Content != "always run make lint before commit" {
		t.Fatalf("after edit = (%+v, %v, %v)", got, ok, err)
	}
	// Editing content clears the now-stale embedding.
	forSearch, _ := store.ItemsForSearch(t.Context(), agentmemory.ScopeProject, "/repo")
	if len(forSearch) != 1 || len(forSearch[0].Embedding) != 0 {
		t.Fatalf("edit did not clear the stale embedding: %+v", forSearch)
	}
	// A combined review update is all-or-nothing: invalid content must not leave
	// a requested pin behind.
	if err := store.SetPinned(t.Context(), item.ID, false, now); err != nil {
		t.Fatal(err)
	}
	pinned := true
	blank := "  "
	if _, err := store.Update(t.Context(), item.ID, &blank, &pinned, now.Add(time.Second)); err == nil {
		t.Fatal("Update accepted blank content")
	}
	unchanged, ok, err := store.Get(t.Context(), item.ID)
	if err != nil || !ok || unchanged.Pinned {
		t.Fatalf("failed Update changed item = (%+v, %v, %v)", unchanged, ok, err)
	}
	if err := store.Delete(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(t.Context(), item.ID); !errors.Is(err, agentmemory.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestAgentMemoryEmbeddingBackfillRoundTrip(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "one", "two")
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(t.Context(), "/repo", 0, facts[1].Sequence, []string{"one", "two"}, now); err != nil {
		t.Fatal(err)
	}
	// Only approved (active) items are embedded; approve the proposals first.
	proposals, _ := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	for _, item := range proposals {
		if err := store.SetStatus(t.Context(), item.ID, agentmemory.StatusActive, now); err != nil {
			t.Fatal(err)
		}
	}

	// Approved items carry no embedding yet.
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
			published, err := store.Reconcile(t.Context(), "/repo", 0, through, []string{"body"}, time.Now())
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
