package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/service/provider"
)

// TestClientResolver_Infers verifies model→provider inference: a cataloged
// model maps to its provider; an empty model uses the runtime default; an
// uncataloged model falls back to the default provider (best effort).
func TestClientResolver_Infers(t *testing.T) {
	r := newClientResolver(provider.NewInMemory(), config.ProviderDeepSeek, "deepseek-v4-flash")

	if p, m := r.resolve("deepseek-v4-pro"); p != config.ProviderDeepSeek || m != "deepseek-v4-pro" {
		t.Errorf("resolve(deepseek-v4-pro) = (%q,%q), want (deepseek, deepseek-v4-pro)", p, m)
	}
	if p, m := r.resolve(""); p != config.ProviderDeepSeek || m != "deepseek-v4-flash" {
		t.Errorf("resolve(\"\") = (%q,%q), want the default (deepseek, deepseek-v4-flash)", p, m)
	}
	// An anthropic model is inferred as anthropic (it's in the catalog) even
	// though the default provider is deepseek.
	if p, _ := r.resolve("claude-3-5-haiku-20241022"); p != config.ProviderAnthropic {
		t.Errorf("resolve(claude…) provider = %q, want anthropic", p)
	}
	// Uncataloged id → default provider, id passed through.
	if p, m := r.resolve("totally-made-up"); p != config.ProviderDeepSeek || m != "totally-made-up" {
		t.Errorf("resolve(unknown) = (%q,%q), want (deepseek, totally-made-up)", p, m)
	}
}

// TestClientResolver_RejectsUnconfigured verifies a model whose provider has
// no key errors out (the "configure it first" path); once configured it
// resolves to a cached client.
func TestClientResolver_RejectsUnconfigured(t *testing.T) {
	ps := provider.NewInMemory() // empty: deepseek not configured
	r := newClientResolver(ps, config.ProviderDeepSeek, "deepseek-v4-flash")

	if _, err := r.ResolveClient(t.Context(), "deepseek-v4-pro"); err == nil {
		t.Fatal("expected an error resolving against an unconfigured provider")
	}

	if err := ps.Configure(t.Context(), provider.Provider{ID: "deepseek", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	c, err := r.ResolveClient(t.Context(), "deepseek-v4-pro")
	if err != nil || c == nil {
		t.Fatalf("ResolveClient after configure: client=%v err=%v", c, err)
	}
	// Same (provider, model) is cached — second call returns the same client.
	if c2, _ := r.ResolveClient(t.Context(), "deepseek-v4-pro"); c2 != c {
		t.Error("expected the resolved client to be cached")
	}
}
