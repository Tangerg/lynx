package modelclient

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

// TestChatResolverRejectsUnconfigured verifies an explicit provider that
// has no key errors out (the "configure it first" path); once configured it
// resolves to a cached client. The provider is taken as given — never
// inferred from the model.
func TestChatResolverRejectsUnconfigured(t *testing.T) {
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := sqlitestore.NewProviderStore(db) // empty: deepseek not configured
	r := NewChatResolver(ps)

	_, err = r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro"))
	if err == nil {
		t.Fatal("expected an error resolving against an unconfigured provider")
	}
	var failure *run.FailureError
	if !errors.As(err, &failure) || failure.Kind != run.FailureInvalidCredentials {
		t.Fatalf("unconfigured provider error = %#v, want invalid-credentials failure", err)
	}

	apiKey := "k"
	_, err = ps.Update(t.Context(), "deepseek", provider.Patch{APIKey: &apiKey})
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro"))
	if err != nil || c == nil {
		t.Fatalf("ResolveChat after configure: client=%v err=%v", c, err)
	}
	// Same (provider, model) is cached — second call returns the same client.
	if c2, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro")); c2 != c {
		t.Error("expected the resolved client to be cached")
	}
	// A different model on the same provider builds a distinct client.
	if c3, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-flash")); c3 == c {
		t.Error("different model should resolve a distinct client")
	}
}

func testDeepSeekSelection(t testing.TB, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New("deepseek", model)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}
