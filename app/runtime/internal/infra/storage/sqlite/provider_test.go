package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func TestProviderStoreUpdatePreservesOmittedFieldsAndClearsExplicitly(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewProviderStore(db)

	apiKey := "sk-old"
	baseURL := "https://old.test"
	got, err := store.Update(t.Context(), "openai", provider.Patch{
		APIKey:  &apiKey,
		BaseURL: &baseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != apiKey || got.BaseURL != baseURL {
		t.Fatalf("initial update = %+v", got)
	}

	replacement := "sk-new"
	got, err = store.Update(t.Context(), "openai", provider.Patch{APIKey: &replacement})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != replacement || got.BaseURL != baseURL {
		t.Fatalf("key-only update = %+v, want endpoint preserved", got)
	}

	clear := ""
	got, err = store.Update(t.Context(), "openai", provider.Patch{BaseURL: &clear})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != replacement || got.BaseURL != "" {
		t.Fatalf("endpoint clear = %+v, want key preserved", got)
	}
}
