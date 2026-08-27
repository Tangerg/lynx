package sqlite_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
)

func newAgentMemoryStore(t *testing.T) *sqlite.AgentMemoryStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
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

func TestAgentMemoryPendingLedgerRequiresBoundedPage(t *testing.T) {
	store := newAgentMemoryStore(t)
	if _, err := store.PendingLedger(
		t.Context(), "/repo", 0, agentmemory.MaxLedgerFoldFacts+1,
	); err == nil {
		t.Fatal("oversized pending-ledger page was accepted")
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
	if active, itemsErr := store.Items(t.Context(), agentmemory.ScopeProject, "/repo"); itemsErr != nil || len(active) != 0 {
		t.Fatalf("active items before approval = (%+v, %v), want none", active, itemsErr)
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
	if err := store.Review(t.Context(), approve.ID, agentmemory.ReviewApprove, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Review(t.Context(), reject.ID, agentmemory.ReviewReject, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Review(t.Context(), approve.ID, agentmemory.ReviewReject, now); !errors.Is(err, agentmemory.ErrNotPending) {
		t.Fatalf("second review = %v, want ErrNotPending", err)
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

func TestAgentMemorySchemaRejectsInvalidDomainVocabulary(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, test := range []struct {
		name   string
		scope  string
		origin string
		status string
	}{
		{name: "scope", scope: "unknown", origin: "auto", status: "pending"},
		{name: "origin", scope: "project", origin: "unknown", status: "pending"},
		{name: "status", scope: "project", origin: "auto", status: "unknown"},
		{name: "user project", scope: "user", origin: "user", status: "active"},
		{name: "user pending", scope: "user", origin: "user", status: "pending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := db.Exec(`INSERT INTO agent_memory_items(
				id, scope, project, content, digest, origin, status, created_at, updated_at
			) VALUES (?, ?, '/repo', 'fact', ?, ?, ?, 1, 1)`,
				"mem_"+test.name, test.scope, "digest_"+test.name, test.origin, test.status)
			if err == nil {
				t.Fatal("invalid agent-memory row was accepted")
			}
		})
	}
}

func TestAgentMemorySchemaRejectsContentBeyondDomainBound(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	oversized := strings.Repeat("界", agentmemory.MaxContentCharacters+1)
	if _, err := db.Exec(`INSERT INTO agent_memory_items(
		id, scope, project, content, digest, origin, status, created_at, updated_at
	) VALUES ('mem_oversized', 'project', '/repo', ?, 'digest', 'user', 'active', 1, 1)`, oversized); err == nil {
		t.Fatal("agent memory item beyond the Domain bound was persisted")
	}
	if _, err := db.Exec(`INSERT INTO agent_memory_ledger(
		project, day, session_id, fact, digest, captured_at
	) VALUES ('/repo', '2026-08-24', 'session', ?, 'digest', 1)`, oversized); err == nil {
		t.Fatal("agent memory ledger fact beyond the Domain bound was persisted")
	}
}

func TestAgentMemoryManagementOps(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)

	item, created, err := store.Add(t.Context(), agentmemory.ScopeProject, "/repo", "always run make lint", now)
	if err != nil || !created || item.ID == "" || item.Origin != agentmemory.OriginUser || item.Status != agentmemory.StatusActive {
		t.Fatalf("add = (%+v, %t, %v)", item, created, err)
	}
	duplicate, duplicateCreated, err := store.Add(
		t.Context(), agentmemory.ScopeProject, "/repo", "always run make lint", now.Add(time.Second),
	)
	if err != nil || duplicateCreated || duplicate.ID != item.ID {
		t.Fatalf("duplicate add = (%+v, %t, %v), want original item without insertion", duplicate, duplicateCreated, err)
	}
	embedding, err := agentmemory.NewEmbeddingUpdate(item, "provider:model", []float32{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if setEmbeddingsErr := store.SetEmbeddings(t.Context(), []agentmemory.EmbeddingUpdate{embedding}); setEmbeddingsErr != nil {
		t.Fatal(setEmbeddingsErr)
	}
	if setPinnedErr := store.SetPinned(t.Context(), item.ID, true, now); setPinnedErr != nil {
		t.Fatal(setPinnedErr)
	}
	if updateContentErr := store.UpdateContent(t.Context(), item.ID, "always run make lint before commit", now); updateContentErr != nil {
		t.Fatal(updateContentErr)
	}
	got, ok, err := store.Get(t.Context(), item.ID)
	if err != nil || !ok || !got.Pinned || got.Content != "always run make lint before commit" {
		t.Fatalf("after edit = (%+v, %v, %v)", got, ok, err)
	}
	// Editing content clears the now-stale embedding.
	forSearch, _ := store.SearchCorpus(t.Context(), "/repo")
	if len(forSearch) != 1 || len(forSearch[0].Embedding) != 0 {
		t.Fatalf("edit did not clear the stale embedding: %+v", forSearch)
	}
	// A combined review update is all-or-nothing: invalid content must not leave
	// a requested pin behind.
	if setPinnedErr := store.SetPinned(t.Context(), item.ID, false, now); setPinnedErr != nil {
		t.Fatal(setPinnedErr)
	}
	pinned := true
	blank := "  "
	if _, updateErr := store.Update(t.Context(), item.ID, &blank, &pinned, now.Add(time.Second)); updateErr == nil {
		t.Fatal("Update accepted blank content")
	}
	unchanged, ok, err := store.Get(t.Context(), item.ID)
	if err != nil || !ok || unchanged.Pinned {
		t.Fatalf("failed Update changed item = (%+v, %v, %v)", unchanged, ok, err)
	}
	oversized := strings.Repeat("界", agentmemory.MaxContentCharacters+1)
	if _, updateErr := store.Update(t.Context(), item.ID, &oversized, &pinned, now.Add(2*time.Second)); updateErr == nil {
		t.Fatal("Update accepted oversized content")
	}
	unchanged, ok, err = store.Get(t.Context(), item.ID)
	if err != nil || !ok || unchanged.Pinned {
		t.Fatalf("oversized Update changed item = (%+v, %v, %v)", unchanged, ok, err)
	}
	if err := store.Delete(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(t.Context(), item.ID); !errors.Is(err, agentmemory.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestAgentMemoryTargetCapacityBoundsCompleteList(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	for index := range 512 {
		if _, created, err := store.Add(
			t.Context(), agentmemory.ScopeProject, "/repo",
			fmt.Sprintf("memory %d", index), now,
		); err != nil || !created {
			t.Fatalf("Add(%d) = (created=%t, err=%v)", index, created, err)
		}
	}
	if _, _, err := store.Add(
		t.Context(), agentmemory.ScopeProject, "/repo", "memory 512", now,
	); !errors.Is(err, agentmemory.ErrTargetFull) {
		t.Fatalf("513th visible item error = %v, want ErrTargetFull", err)
	}
	items, err := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != 512 {
		t.Fatalf("List = (%d items, %v), want the complete 512-item target", len(items), err)
	}
	if _, created, err := store.Add(
		t.Context(), agentmemory.ScopeProject, "/other", "independent target", now,
	); err != nil || !created {
		t.Fatalf("independent target Add = (created=%t, err=%v)", created, err)
	}
}

func TestAgentMemoryReconcilePublishesOnlyAvailableTargetCapacity(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	for index := range agentmemory.MaxVisiblePerTarget - 1 {
		if _, _, err := store.Add(
			t.Context(), agentmemory.ScopeProject, "/repo",
			fmt.Sprintf("accepted memory %d", index), now,
		); err != nil {
			t.Fatal(err)
		}
	}
	facts := appendAgentFacts(t, store, "/repo", "2026-08-24", "ledger fact")
	published, err := store.Reconcile(
		t.Context(), "/repo", 0, facts[0].Sequence,
		[]string{"highest priority proposal", "lower priority proposal"}, now.Add(time.Second),
	)
	if err != nil || !published {
		t.Fatalf("Reconcile = (%t, %v)", published, err)
	}
	items, err := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != agentmemory.MaxVisiblePerTarget {
		t.Fatalf("List = (%d items, %v)", len(items), err)
	}
	if items[0].Content != "highest priority proposal" || items[0].Status != agentmemory.StatusPending {
		t.Fatalf("capacity proposal = %+v", items[0])
	}
	if slices.ContainsFunc(items, func(item agentmemory.Item) bool {
		return item.Content == "lower priority proposal"
	}) {
		t.Fatal("lower-priority proposal exceeded the target capacity")
	}
	state, err := store.State(t.Context(), "/repo")
	if err != nil || state.Watermark != facts[0].Sequence {
		t.Fatalf("state = (%+v, %v)", state, err)
	}
}

func TestAgentMemoryListRejectsCorruptOverfullTarget(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 513 {
		_, err = tx.ExecContext(t.Context(), `INSERT INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, created_at, updated_at
		) VALUES (?, 'project', '/repo', ?, ?, 'user', 'active', 1, 1)`,
			fmt.Sprintf("mem_%d", index), fmt.Sprintf("memory %d", index), fmt.Sprintf("digest_%d", index))
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewAgentMemoryStore(db)
	queries := []struct {
		name string
		read func() ([]agentmemory.Item, error)
	}{
		{name: "management list", read: func() ([]agentmemory.Item, error) {
			return store.List(t.Context(), agentmemory.ScopeProject, "/repo")
		}},
		{name: "prompt items", read: func() ([]agentmemory.Item, error) {
			return store.Items(t.Context(), agentmemory.ScopeProject, "/repo")
		}},
		{name: "search corpus", read: func() ([]agentmemory.Item, error) {
			return store.SearchCorpus(t.Context(), "/repo")
		}},
	}
	for _, query := range queries {
		if items, err := query.read(); err == nil {
			t.Errorf("%s returned %d overfull items without an error", query.name, len(items))
		}
	}
}

func TestAgentMemoryExplicitAddRevivesRejectedProposal(t *testing.T) {
	store := newAgentMemoryStore(t)
	facts := appendAgentFacts(t, store, "/repo", "2026-08-24", "revive me")
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	if _, err := store.Reconcile(
		t.Context(), "/repo", 0, facts[0].Sequence, []string{"revive me"}, now,
	); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != 1 {
		t.Fatalf("proposal list = (%+v, %v)", items, err)
	}
	if reviewErr := store.Review(t.Context(), items[0].ID, agentmemory.ReviewReject, now.Add(time.Second)); reviewErr != nil {
		t.Fatal(reviewErr)
	}
	revived, created, err := store.Add(
		t.Context(), agentmemory.ScopeProject, "/repo", "revive me", now.Add(2*time.Second),
	)
	if err != nil || !created || revived.ID != items[0].ID ||
		revived.Origin != agentmemory.OriginUser || revived.Status != agentmemory.StatusActive {
		t.Fatalf("revived Add = (%+v, created=%t, err=%v)", revived, created, err)
	}
}

func TestAgentMemoryReviewBoundsRejectedTombstones(t *testing.T) {
	const maximumRejected = 2048
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range maximumRejected {
		_, err = tx.ExecContext(t.Context(), `INSERT INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, created_at, updated_at
		) VALUES (?, 'project', '/repo', ?, ?, 'auto', 'rejected', 1, ?)`,
			fmt.Sprintf("mem_old_%d", index), fmt.Sprintf("old rejection %d", index),
			fmt.Sprintf("old_digest_%d", index), index+1)
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	_, err = tx.ExecContext(t.Context(), `INSERT INTO agent_memory_items(
		id, scope, project, content, digest, origin, status, created_at, updated_at
	) VALUES ('mem_new', 'project', '/repo', 'new rejection', 'new_digest', 'auto', 'pending', 1, 1)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewAgentMemoryStore(db)
	if err := store.Review(
		t.Context(), "mem_new", agentmemory.ReviewReject,
		time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatal(err)
	}
	var rejected int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM agent_memory_items
		WHERE scope = 'project' AND project = '/repo' AND status = 'rejected'`).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected != maximumRejected {
		t.Fatalf("rejected tombstones = %d, want %d", rejected, maximumRejected)
	}
	var preserved int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM agent_memory_items
		WHERE id = 'mem_new' AND status = 'rejected'`).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("new rejection preserved = %d, err=%v", preserved, err)
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
		if err := store.Review(t.Context(), item.ID, agentmemory.ReviewApprove, now); err != nil {
			t.Fatal(err)
		}
	}

	// Approved items carry no embedding yet.
	forSearch, err := store.SearchCorpus(t.Context(), "/repo")
	if err != nil || len(forSearch) != 2 {
		t.Fatalf("items for search = (%+v, %v), want 2", forSearch, err)
	}
	updates := make([]agentmemory.EmbeddingUpdate, 0, len(forSearch))
	vectors := make(map[string][]float32, len(forSearch))
	for i, item := range forSearch {
		vector := []float32{float32(i + 1), 0.5}
		vectors[item.ID] = vector
		update, updateErr := agentmemory.NewEmbeddingUpdate(item, "provider:model", vector)
		if updateErr != nil {
			t.Fatal(updateErr)
		}
		updates = append(updates, update)
	}
	if setEmbeddingsErr := store.SetEmbeddings(t.Context(), updates); setEmbeddingsErr != nil {
		t.Fatal(setEmbeddingsErr)
	}

	// The search fetch decodes both the vector and its exact space identity.
	forSearch, err = store.SearchCorpus(t.Context(), "/repo")
	if err != nil || len(forSearch) != 2 {
		t.Fatalf("items for search = (%+v, %v)", forSearch, err)
	}
	for _, item := range forSearch {
		want := vectors[item.ID]
		if item.EmbeddingSpace != "provider:model" || len(item.Embedding) != len(want) || item.Embedding[0] != want[0] || item.Embedding[1] != want[1] {
			t.Fatalf("embedding round-trip failed for %s: got %v want %v", item.ID, item.Embedding, want)
		}
	}
}

func TestAgentMemorySearchCorpusIncludesUserAndExactProject(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	projectItem, _, err := store.Add(t.Context(), agentmemory.ScopeProject, "/repo", "project convention", now)
	if err != nil {
		t.Fatal(err)
	}
	userItem, _, err := store.Add(t.Context(), agentmemory.ScopeUser, "", "user preference", now)
	if err != nil {
		t.Fatal(err)
	}
	otherItem, _, err := store.Add(t.Context(), agentmemory.ScopeProject, "/other", "other project convention", now)
	if err != nil {
		t.Fatal(err)
	}

	corpus, err := store.SearchCorpus(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]agentmemory.Scope, len(corpus))
	for _, item := range corpus {
		got[item.ID] = item.Scope
	}
	if len(got) != 2 || got[projectItem.ID] != agentmemory.ScopeProject || got[userItem.ID] != agentmemory.ScopeUser {
		t.Fatalf("search corpus = %+v, want exact project + user items", corpus)
	}
	if _, leaked := got[otherItem.ID]; leaked {
		t.Fatalf("other-project item leaked into search corpus: %+v", corpus)
	}
}

func TestAgentMemoryLateEmbeddingDoesNotOverwriteEditedContent(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	item, _, err := store.Add(t.Context(), agentmemory.ScopeProject, "/repo", "old content", now)
	if err != nil {
		t.Fatal(err)
	}
	late, err := agentmemory.NewEmbeddingUpdate(item, "provider:model", []float32{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if updateContentErr := store.UpdateContent(t.Context(), item.ID, "new content", now.Add(time.Second)); updateContentErr != nil {
		t.Fatal(updateContentErr)
	}
	if setEmbeddingsErr := store.SetEmbeddings(t.Context(), []agentmemory.EmbeddingUpdate{late}); setEmbeddingsErr != nil {
		t.Fatal(setEmbeddingsErr)
	}
	items, err := store.SearchCorpus(t.Context(), "/repo")
	if err != nil || len(items) != 1 {
		t.Fatalf("items for search = (%+v, %v)", items, err)
	}
	if items[0].Content != "new content" || items[0].EmbeddingSpace != "" || len(items[0].Embedding) != 0 {
		t.Fatalf("late embedding polluted edited item: %+v", items[0])
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

func TestAgentMemoryConcurrentAddReportsOneCreation(t *testing.T) {
	store := newAgentMemoryStore(t)
	var created atomic.Int32
	var ids sync.Map
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			item, inserted, err := store.Add(
				t.Context(), agentmemory.ScopeProject, "/repo", "same memory", time.Now(),
			)
			if err != nil {
				t.Errorf("Add: %v", err)
				return
			}
			ids.Store(item.ID, struct{}{})
			if inserted {
				created.Add(1)
			}
		})
	}
	wg.Wait()
	if got := created.Load(); got != 1 {
		t.Fatalf("created results = %d, want 1", got)
	}
	var distinct int
	ids.Range(func(_, _ any) bool {
		distinct++
		return true
	})
	if distinct != 1 {
		t.Fatalf("distinct item ids = %d, want 1", distinct)
	}
}

func TestAgentMemoryConcurrentAddsCannotExceedTargetCapacity(t *testing.T) {
	store := newAgentMemoryStore(t)
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	for index := range agentmemory.MaxVisiblePerTarget - 1 {
		if _, _, err := store.Add(
			t.Context(), agentmemory.ScopeProject, "/repo", fmt.Sprintf("existing %d", index), now,
		); err != nil {
			t.Fatal(err)
		}
	}
	var created atomic.Int32
	var wg sync.WaitGroup
	for index := range 8 {
		wg.Go(func() {
			_, inserted, err := store.Add(
				t.Context(), agentmemory.ScopeProject, "/repo",
				fmt.Sprintf("concurrent %d", index), now.Add(time.Second),
			)
			if errors.Is(err, agentmemory.ErrTargetFull) {
				return
			}
			if err != nil {
				t.Errorf("Add: %v", err)
				return
			}
			if !inserted {
				t.Error("unique concurrent Add was reported as idempotent")
				return
			}
			created.Add(1)
		})
	}
	wg.Wait()
	if got := created.Load(); got != 1 {
		t.Fatalf("created = %d, want exactly one remaining slot winner", got)
	}
	items, err := store.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != agentmemory.MaxVisiblePerTarget {
		t.Fatalf("List = (%d items, %v)", len(items), err)
	}
}
