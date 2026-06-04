package storage_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

// TestFileHistoryStore_RoundTrip covers the contract items.list depends
// on: items come back in append order with their blobs intact, and a
// RunRef upserts (run.started → run.finished) to one row, scoped per
// session.
func TestFileHistoryStore_RoundTrip(t *testing.T) {
	withTempHome(t)
	store, err := storage.NewFileHistoryStore()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	// Two items for ses_a (append order), one for ses_b (isolation).
	mustAppend(t, store, history.Item{SessionID: "ses_a", RunID: "run_1", ItemID: "i1", CreatedAt: now, Blob: json.RawMessage(`{"id":"i1"}`)})
	mustAppend(t, store, history.Item{SessionID: "ses_a", RunID: "run_1", ItemID: "i2", CreatedAt: now, Blob: json.RawMessage(`{"id":"i2"}`)})
	mustAppend(t, store, history.Item{SessionID: "ses_b", RunID: "run_9", ItemID: "i9", CreatedAt: now, Blob: json.RawMessage(`{"id":"i9"}`)})

	// run_1 starts then finishes — second PutRun must replace, not append.
	if err := store.PutRun(ctx, history.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"running"}`)}); err != nil {
		t.Fatalf("put run (running): %v", err)
	}
	if err := store.PutRun(ctx, history.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"finished"}`)}); err != nil {
		t.Fatalf("put run (finished): %v", err)
	}

	items, runs, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ItemID != "i1" || items[1].ItemID != "i2" {
		t.Fatalf("items = %+v, want [i1 i2] in order", items)
	}
	if len(runs) != 1 || string(runs[0].Blob) != `{"status":"finished"}` {
		t.Fatalf("runs = %+v, want one upserted finished run", runs)
	}

	// ses_b stayed isolated.
	bItems, _, err := store.List(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if len(bItems) != 1 || bItems[0].ItemID != "i9" {
		t.Fatalf("ses_b items = %+v, want [i9]", bItems)
	}
}

func mustAppend(t *testing.T, store history.Store, it history.Item) {
	t.Helper()
	if err := store.AppendItem(context.Background(), it); err != nil {
		t.Fatalf("append %s: %v", it.ItemID, err)
	}
}
