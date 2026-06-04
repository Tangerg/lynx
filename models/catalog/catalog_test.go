package catalog_test

import (
	"testing"

	"github.com/Tangerg/lynx/models/catalog"
)

func TestModels(t *testing.T) {
	models := catalog.Models("anthropic")
	if len(models) == 0 {
		t.Fatal("anthropic should have cataloged models")
	}
	// Case-insensitive on the provider name (adapter consts are capitalized).
	if len(catalog.Models("Anthropic")) != len(models) {
		t.Fatal("provider name must match case-insensitively")
	}
	if catalog.Models("does-not-exist") != nil {
		t.Fatal("unknown provider must return nil")
	}
}

func TestLookup(t *testing.T) {
	info, ok := catalog.Lookup("anthropic", "claude-3-5-haiku-20241022")
	if !ok {
		t.Fatal("known model must be found")
	}
	if len(info.Pricing) == 0 {
		t.Fatal("model info must carry pricing")
	}
	if _, ok := catalog.Lookup("anthropic", "no-such-model"); ok {
		t.Fatal("unknown model must report ok=false")
	}
}

func TestGet(t *testing.T) {
	p, ok := catalog.Get("anthropic")
	if !ok || p.ID != "anthropic" || len(p.Models) == 0 {
		t.Fatalf("Get(anthropic) = %+v, %v", p, ok)
	}
	if _, ok := catalog.Get("does-not-exist"); ok {
		t.Fatal("Get of unknown provider must report ok=false")
	}
}
