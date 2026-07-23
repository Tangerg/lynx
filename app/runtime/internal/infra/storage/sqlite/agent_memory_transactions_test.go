package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

func TestAgentMemoryQueriesJoinCallerTransaction(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewAgentMemoryStore(db)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if _, err := store.Add(ctx, agentmemory.ScopeProject, "/repo", "remember this", time.Now()); err != nil {
		t.Fatal(err)
	}
	queries := []struct {
		name string
		list func(context.Context) ([]agentmemory.Item, error)
	}{
		{name: "items", list: func(ctx context.Context) ([]agentmemory.Item, error) {
			return store.Items(ctx, agentmemory.ScopeProject, "/repo")
		}},
		{name: "items for search", list: func(ctx context.Context) ([]agentmemory.Item, error) {
			return store.ItemsForSearch(ctx, agentmemory.ScopeProject, "/repo")
		}},
		{name: "unembedded items", list: func(ctx context.Context) ([]agentmemory.Item, error) {
			return store.UnembeddedItems(ctx, agentmemory.ScopeProject, "/repo")
		}},
		{name: "review list", list: func(ctx context.Context) ([]agentmemory.Item, error) {
			return store.List(ctx, agentmemory.ScopeProject, "/repo")
		}},
	}

	if err := RunInTx(ctx, db, func(ctx context.Context) error {
		for _, query := range queries {
			items, err := query.list(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", query.name, err)
			}
			if len(items) != 1 {
				return fmt.Errorf("%s items = %d, want 1", query.name, len(items))
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
}
