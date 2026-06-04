package provider_test

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/provider"
)

func TestInMemory_ConfigureListGet(t *testing.T) {
	ctx := t.Context()
	s := provider.NewInMemory()

	// Unconfigured: not present.
	if _, ok, _ := s.Get(ctx, "deepseek"); ok {
		t.Fatal("empty registry should not contain deepseek")
	}

	// Configure two; List is sorted by id.
	if err := s.Configure(ctx, provider.Provider{ID: "openai", APIKey: "sk-o"}); err != nil {
		t.Fatalf("configure openai: %v", err)
	}
	if err := s.Configure(ctx, provider.Provider{ID: "deepseek", BaseURL: "https://x"}); err != nil {
		t.Fatalf("configure deepseek: %v", err)
	}
	list, _ := s.List(ctx)
	if len(list) != 2 || list[0].ID != "deepseek" || list[1].ID != "openai" {
		t.Fatalf("List = %+v, want [deepseek, openai]", list)
	}

	// Enabled is key-derived.
	if list[0].Enabled() {
		t.Error("deepseek has no key — must not be enabled")
	}
	if !list[1].Enabled() {
		t.Error("openai has a key — must be enabled")
	}

	// Configure is an upsert.
	if err := s.Configure(ctx, provider.Provider{ID: "deepseek", APIKey: "sk-d"}); err != nil {
		t.Fatalf("reconfigure: %v", err)
	}
	got, ok, _ := s.Get(ctx, "deepseek")
	if !ok || got.APIKey != "sk-d" || !got.Enabled() {
		t.Fatalf("after upsert deepseek = %+v", got)
	}
}
