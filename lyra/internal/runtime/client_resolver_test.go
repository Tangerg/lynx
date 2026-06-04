package runtime

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/provider"
	sqlitestore "github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
)

// TestClientResolver_RejectsUnconfigured verifies an explicit provider that
// has no key errors out (the "configure it first" path); once configured it
// resolves to a cached client. The provider is taken as given — never
// inferred from the model.
func TestClientResolver_RejectsUnconfigured(t *testing.T) {
	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := sqlitestore.NewProviderService(db) // empty: deepseek not configured
	r := newClientResolver(ps)

	if _, err := r.ResolveClient(t.Context(), "deepseek", "deepseek-v4-pro"); err == nil {
		t.Fatal("expected an error resolving against an unconfigured provider")
	}

	if err := ps.Configure(t.Context(), provider.Provider{ID: "deepseek", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	c, err := r.ResolveClient(t.Context(), "deepseek", "deepseek-v4-pro")
	if err != nil || c == nil {
		t.Fatalf("ResolveClient after configure: client=%v err=%v", c, err)
	}
	// Same (provider, model) is cached — second call returns the same client.
	if c2, _ := r.ResolveClient(t.Context(), "deepseek", "deepseek-v4-pro"); c2 != c {
		t.Error("expected the resolved client to be cached")
	}
	// A different model on the same provider builds a distinct client.
	if c3, _ := r.ResolveClient(t.Context(), "deepseek", "deepseek-v4-flash"); c3 == c {
		t.Error("different model should resolve a distinct client")
	}
}
