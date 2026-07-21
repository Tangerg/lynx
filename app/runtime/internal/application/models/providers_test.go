package models

import (
	"context"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

type fakeRegistry struct {
	entry provider.Provider
	ok    bool
}

func (f fakeRegistry) List(context.Context) ([]provider.Provider, error) { return nil, nil }
func (f fakeRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	if f.ok && f.entry.ID == id {
		return f.entry, true, nil
	}
	return provider.Provider{}, false, nil
}
func (f fakeRegistry) Configure(context.Context, provider.Provider) error { return nil }

type fakeLister struct {
	gotEntry provider.Provider
	ids      []string
	err      error
}

func (f *fakeLister) ListModels(_ context.Context, entry provider.Provider) ([]string, error) {
	f.gotEntry = entry
	return f.ids, f.err
}

func TestListRemoteModelsPassesConfiguredEntry(t *testing.T) {
	reg := fakeRegistry{entry: provider.Provider{ID: "ollama", BaseURL: "http://host:1234/v1", APIKey: "k"}, ok: true}
	lister := &fakeLister{ids: []string{"a", "b"}}
	c := New(Config{Providers: reg, Lister: lister})

	ids, err := c.ListRemoteModels(t.Context(), "ollama")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"a", "b"}) {
		t.Fatalf("ids = %v, want [a b]", ids)
	}
	if lister.gotEntry.BaseURL != "http://host:1234/v1" || lister.gotEntry.APIKey != "k" {
		t.Fatalf("lister received entry %+v, want configured base URL + key", lister.gotEntry)
	}
}

func TestListRemoteModelsUnconfiguredProviderStillProbes(t *testing.T) {
	// An Ollama daemon can be probed on its localhost default without any
	// registry entry — the coordinator hands the lister a bare {ID} entry.
	lister := &fakeLister{ids: []string{"local-model"}}
	c := New(Config{Providers: fakeRegistry{ok: false}, Lister: lister})

	ids, err := c.ListRemoteModels(t.Context(), "ollama")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"local-model"}) {
		t.Fatalf("ids = %v, want [local-model]", ids)
	}
	if lister.gotEntry.ID != "ollama" {
		t.Fatalf("lister received entry id %q, want ollama", lister.gotEntry.ID)
	}
}

func TestListRemoteModelsNilListerIsNoOp(t *testing.T) {
	c := New(Config{Providers: fakeRegistry{}})
	ids, err := c.ListRemoteModels(t.Context(), "ollama")
	if err != nil || ids != nil {
		t.Fatalf("ids=%v err=%v, want nil,nil (static-catalog fallback)", ids, err)
	}
}
