package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// stubCatalog reports a single provider's metadata (only ProbeModels matters to
// ListModels' merge branch).
type stubCatalog struct {
	meta provider.Metadata
}

func (s stubCatalog) Supported() []provider.Metadata { return []provider.Metadata{s.meta} }
func (s stubCatalog) Metadata(id string) (provider.Metadata, bool) {
	if s.meta.ID == id {
		return s.meta, true
	}
	return provider.Metadata{}, false
}
func (stubCatalog) Models(string) []models.Model { return nil }
func (stubCatalog) LookupModel(string, string) (models.Model, bool) {
	return models.Model{}, false
}

// stubLister records whether the probe ran and returns canned ids/err.
type stubLister struct {
	ids   []string
	err   error
	calls int
}

func (s *stubLister) ListModels(context.Context, provider.Provider) ([]string, error) {
	s.calls++
	return s.ids, s.err
}

type stubRegistry struct{}

func (stubRegistry) List(context.Context) ([]provider.Provider, error) { return nil, nil }
func (stubRegistry) Get(context.Context, string) (provider.Provider, bool, error) {
	return provider.Provider{}, false, nil
}
func (stubRegistry) Configure(context.Context, provider.Provider) error { return nil }

func probeServer(meta provider.Metadata, lister models.ProviderModelLister) *Server {
	return serverWithModels(models.Config{Providers: stubRegistry{}, Catalog: stubCatalog{meta: meta}, Lister: lister})
}

func listModels(t *testing.T, s *Server, providerID string) []protocol.Model {
	t.Helper()
	page, err := s.ListModels(t.Context(), protocol.ListModelsRequest{Provider: providerID})
	if err != nil {
		t.Fatalf("ListModels error = %v", err)
	}
	return page.Data
}

// "testprov" has no entry in the static catalog, so the static-fallback paths
// yield an empty page — letting each test assert the branch it exercises without
// coupling to the embedded catalog's contents.

func TestListModelsProbesEndpointAuthoritativeProvider(t *testing.T) {
	lister := &stubLister{ids: []string{"m-alpha", "m-beta"}}
	got := listModels(t, probeServer(provider.Metadata{ID: "testprov", ProbeModels: true}, lister), "testprov")
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1", lister.calls)
	}
	if len(got) != 2 || got[0].ID != "m-alpha" || got[1].ID != "m-beta" {
		t.Fatalf("models = %+v, want the probed ids", got)
	}
	if got[0].Provider != "testprov" {
		t.Fatalf("probed model provider = %q, want testprov", got[0].Provider)
	}
}

func TestListModelsFallsBackWhenProbeEmpty(t *testing.T) {
	lister := &stubLister{ids: nil}
	got := listModels(t, probeServer(provider.Metadata{ID: "testprov", ProbeModels: true}, lister), "testprov")
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1 (probe attempted)", lister.calls)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want empty (static-catalog fallback)", got)
	}
}

func TestListModelsFallsBackWhenProbeErrors(t *testing.T) {
	lister := &stubLister{err: errors.New("unreachable")}
	got := listModels(t, probeServer(provider.Metadata{ID: "testprov", ProbeModels: true}, lister), "testprov")
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1", lister.calls)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want empty; a probe error must not surface as an RPC error", got)
	}
}

func TestListModelsSkipsProbeForCatalogProvider(t *testing.T) {
	lister := &stubLister{ids: []string{"should-not-appear"}}
	got := listModels(t, probeServer(provider.Metadata{ID: "testprov", ProbeModels: false}, lister), "testprov")
	if lister.calls != 0 {
		t.Fatalf("lister calls = %d, want 0 (catalog provider must not be probed)", lister.calls)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want empty (static catalog)", got)
	}
}
