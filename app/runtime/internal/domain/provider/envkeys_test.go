package provider

import (
	"context"
	"testing"
)

// fakeRegistry is an in-memory provider.Registry for exercising the env decorator.
type fakeRegistry struct {
	stored map[string]Provider
}

func (f *fakeRegistry) List(context.Context) ([]Provider, error) {
	out := make([]Provider, 0, len(f.stored))
	for _, p := range f.stored {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRegistry) Get(_ context.Context, id string) (Provider, bool, error) {
	p, ok := f.stored[id]
	return p, ok, nil
}

func (f *fakeRegistry) Configure(_ context.Context, p Provider) error {
	f.stored[p.ID] = p
	return nil
}

func TestWithEnvKeys_StoredWinsOverEnv(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-stored", BaseURL: "https://x"},
	}}
	svc := WithEnvKeys(inner, map[string]string{"anthropic": "sk-env"})

	got, ok, err := svc.Get(context.Background(), "anthropic")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.APIKey != "sk-stored" {
		t.Errorf("APIKey = %q, want stored to win", got.APIKey)
	}
	if got.KeySource != KeyStored {
		t.Errorf("KeySource = %q, want %q", got.KeySource, KeyStored)
	}
	if got.BaseURL != "https://x" {
		t.Errorf("BaseURL = %q, want preserved", got.BaseURL)
	}
}

func TestWithEnvKeys_EnvOnlyProviderIsEnabled(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]Provider{}}
	svc := WithEnvKeys(inner, map[string]string{"openai": "sk-env"})

	got, ok, err := svc.Get(context.Background(), "openai")
	if err != nil || !ok {
		t.Fatalf("Get env-only: ok=%v err=%v", ok, err)
	}
	if !got.Enabled() {
		t.Error("env-only provider should be enabled")
	}
	if got.KeySource != KeyEnv {
		t.Errorf("KeySource = %q, want %q", got.KeySource, KeyEnv)
	}
	if got.APIKey != "sk-env" {
		t.Errorf("APIKey = %q, want env key", got.APIKey)
	}
}

func TestWithEnvKeys_StoredEmptyFallsBackToEnvKeepsBaseURL(t *testing.T) {
	// A row with a base URL but no key (e.g. left over from a cleared key)
	// falls back to env while keeping the configured endpoint.
	inner := &fakeRegistry{stored: map[string]Provider{
		"deepseek": {ID: "deepseek", APIKey: "", BaseURL: "https://ep"},
	}}
	svc := WithEnvKeys(inner, map[string]string{"deepseek": "sk-env"})

	got, _, _ := svc.Get(context.Background(), "deepseek")
	if got.KeySource != KeyEnv || got.APIKey != "sk-env" || got.BaseURL != "https://ep" {
		t.Errorf("got %+v, want env key with base URL preserved", got)
	}
}

func TestWithEnvKeys_UnconfiguredStaysNone(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]Provider{}}
	svc := WithEnvKeys(inner, map[string]string{"openai": "sk-env"})

	got, ok, _ := svc.Get(context.Background(), "groq")
	if ok || got.Enabled() {
		t.Errorf("unknown provider should be neither found nor enabled, got ok=%v %+v", ok, got)
	}
}

func TestWithEnvKeys_ListMergesEnvOnlyAndSorts(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]Provider{
		"openai": {ID: "openai", APIKey: "sk-stored"},
	}}
	svc := WithEnvKeys(inner, map[string]string{
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
	if list[0].KeySource != KeyEnv {
		t.Errorf("anthropic KeySource = %q, want env", list[0].KeySource)
	}
	if list[1].KeySource != KeyStored {
		t.Errorf("openai KeySource = %q, want stored (stored>env)", list[1].KeySource)
	}
}

func TestWithEnvKeys_EmptyMapIsPassThrough(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]Provider{}}
	if got := WithEnvKeys(inner, nil); got != Registry(inner) {
		t.Error("empty env map should return inner unchanged")
	}
}
