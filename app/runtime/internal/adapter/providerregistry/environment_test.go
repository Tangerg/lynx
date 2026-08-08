package providerregistry

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// fakeRegistry is an in-memory provider.Registry for exercising the env decorator.
type fakeRegistry struct {
	stored map[string]provider.Provider
}

func (f *fakeRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(f.stored))
	for _, p := range f.stored {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	p, ok := f.stored[id]
	return p, ok, nil
}

func (f *fakeRegistry) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	p := f.stored[id]
	p.ID = id
	p = p.Apply(patch)
	f.stored[id] = p
	return p, nil
}

func TestWithEnvKeys_StoredWinsOverEnv(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-stored", BaseURL: "https://x"},
	}}
	svc := WithEnvironmentKeys(inner, map[string]string{"anthropic": "sk-env"})

	got, ok, err := svc.Get(context.Background(), "anthropic")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.APIKey != "sk-stored" {
		t.Errorf("APIKey = %q, want stored to win", got.APIKey)
	}
	if got.KeySource != provider.KeyStored {
		t.Errorf("KeySource = %q, want %q", got.KeySource, provider.KeyStored)
	}
	if got.BaseURL != "https://x" {
		t.Errorf("BaseURL = %q, want preserved", got.BaseURL)
	}
}

func TestWithEnvKeys_EnvOnlyProviderIsEnabled(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{}}
	svc := WithEnvironmentKeys(inner, map[string]string{"openai": "sk-env"})

	got, ok, err := svc.Get(context.Background(), "openai")
	if err != nil || !ok {
		t.Fatalf("Get env-only: ok=%v err=%v", ok, err)
	}
	if !got.Enabled() {
		t.Error("env-only provider should be enabled")
	}
	if got.KeySource != provider.KeyEnv {
		t.Errorf("KeySource = %q, want %q", got.KeySource, provider.KeyEnv)
	}
	if got.APIKey != "sk-env" {
		t.Errorf("APIKey = %q, want env key", got.APIKey)
	}
}

func TestWithEnvKeys_StoredEmptyFallsBackToEnvKeepsBaseURL(t *testing.T) {
	// A row with a base URL but no key (e.g. left over from a cleared key)
	// falls back to env while keeping the configured endpoint.
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"deepseek": {ID: "deepseek", APIKey: "", BaseURL: "https://ep"},
	}}
	svc := WithEnvironmentKeys(inner, map[string]string{"deepseek": "sk-env"})

	got, _, _ := svc.Get(context.Background(), "deepseek")
	if got.KeySource != provider.KeyEnv || got.APIKey != "sk-env" || got.BaseURL != "https://ep" {
		t.Errorf("got %+v, want env key with base URL preserved", got)
	}
}

func TestWithEnvKeys_UpdateNeverPersistsEnvironmentKey(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"deepseek": {ID: "deepseek", APIKey: "sk-stored", BaseURL: "https://old"},
	}}
	svc := WithEnvironmentKeys(inner, map[string]string{"deepseek": "sk-env"})

	baseURL := "https://new"
	got, err := svc.Update(t.Context(), "deepseek", provider.Patch{BaseURL: &baseURL})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-stored" || inner.stored["deepseek"].APIKey != "sk-stored" {
		t.Fatalf("base URL update changed key: effective=%+v stored=%+v", got, inner.stored["deepseek"])
	}

	clear := ""
	got, err = svc.Update(t.Context(), "deepseek", provider.Patch{APIKey: &clear})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "sk-env" || got.KeySource != provider.KeyEnv {
		t.Fatalf("cleared stored key resolved as %+v, want env fallback", got)
	}
	if inner.stored["deepseek"].APIKey != "" {
		t.Fatalf("stored key = %q, want cleared without persisting env", inner.stored["deepseek"].APIKey)
	}
}

func TestWithEnvKeys_UnconfiguredStaysNone(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{}}
	svc := WithEnvironmentKeys(inner, map[string]string{"openai": "sk-env"})

	got, ok, _ := svc.Get(context.Background(), "groq")
	if ok || got.Enabled() {
		t.Errorf("unknown provider should be neither found nor enabled, got ok=%v %+v", ok, got)
	}
}

func TestWithEnvKeys_ListMergesEnvOnlyAndSorts(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"openai": {ID: "openai", APIKey: "sk-stored"},
	}}
	svc := WithEnvironmentKeys(inner, map[string]string{
		"anthropic": "sk-env", // env-only, must appear
		"openai":    "sk-env", // stored wins, must not duplicate
	})

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2 (no duplicate openai), got %+v", len(list), list)
	}
	if list[0].ID != "anthropic" || list[1].ID != "openai" {
		t.Errorf("not sorted by id: %+v", list)
	}
	if list[0].KeySource != provider.KeyEnv {
		t.Errorf("anthropic KeySource = %q, want env", list[0].KeySource)
	}
	if list[1].KeySource != provider.KeyStored {
		t.Errorf("openai KeySource = %q, want stored (stored>env)", list[1].KeySource)
	}
}

func TestWithEnvKeys_EmptyMapStillProjectsStoredSource(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"openai": {ID: "openai", APIKey: "sk-stored"},
	}}
	registry := WithEnvironmentKeys(inner, nil)
	got, ok, err := registry.Get(t.Context(), "openai")
	if err != nil || !ok || got.KeySource != provider.KeyStored {
		t.Fatalf("stored provider = %+v, ok=%v, err=%v", got, ok, err)
	}
}

func TestWithEnvKeys_SnapshotsInput(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{}}
	env := map[string]string{"openai": "before"}
	svc := WithEnvironmentKeys(inner, env)
	env["openai"] = "after"

	got, ok, err := svc.Get(context.Background(), "openai")
	if err != nil || !ok || got.APIKey != "before" {
		t.Fatalf("Get after caller mutation = %+v, ok=%v, err=%v", got, ok, err)
	}
}
