package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// fakeCodebaseIndex is the semantic-index capability the codebase coordinator
// drives; the codebase wire handlers are tested against it.
type fakeCodebaseIndex struct {
	available    bool
	availableErr error
	hits         []codebaseindex.Hit
	status       codebaseindex.Status
	searchRoot   string
	searchQuery  string
	searchLimit  int
	statusRoot   string
	reindexed    chan string
}

func (i *fakeCodebaseIndex) Available(context.Context) (bool, error) {
	return i.available, i.availableErr
}

func (i *fakeCodebaseIndex) Reindex(_ context.Context, root string) error {
	i.reindexed <- root
	return nil
}

func (i *fakeCodebaseIndex) Search(_ context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	i.searchRoot = root
	i.searchQuery = query
	i.searchLimit = limit
	return i.hits, nil
}

func (i *fakeCodebaseIndex) Status(_ context.Context, root string) (codebaseindex.Status, error) {
	i.statusRoot = root
	return i.status, nil
}

func serverWithCodebase(root string, idx codebase.Index) *Server {
	surfaces := newWorkspaceSurfaces(root, workspaceTestConfig{})
	s := &Server{codebase: codebase.New(idx, surfaces.roots)}
	applyWorkspaceSurfaces(s, surfaces)
	return s
}

func TestCodebaseSearchMapsToWire(t *testing.T) {
	root := t.TempDir()
	idx := &fakeCodebaseIndex{
		available: true,
		hits: []codebaseindex.Hit{{
			Path:      "runtime/session.go",
			StartLine: 10,
			EndLine:   12,
			Text:      "type Session struct{}",
			Score:     0.9,
		}},
	}
	s := serverWithCodebase(root, idx)

	got, err := s.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{Query: "session", Limit: 3})
	if err != nil {
		t.Fatalf("codebase search: %v", err)
	}
	if idx.searchRoot != workspacepath.Canonical(root) || idx.searchQuery != "session" || idx.searchLimit != 3 {
		t.Fatalf("search root=%q query=%q limit=%d", idx.searchRoot, idx.searchQuery, idx.searchLimit)
	}
	if len(got.Hits) != 1 || got.Hits[0].Path != "runtime/session.go" || got.Hits[0].Score != 0.9 {
		t.Fatalf("wire hits = %+v", got.Hits)
	}
}

func TestCodebaseSearchRequiresIndexAndQuery(t *testing.T) {
	root := t.TempDir()
	s := serverWithCodebase(root, nil) // no index

	_, err := s.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{Query: "session"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("search without index err = %v, want invalid_params", err)
	}

	withIndex := serverWithCodebase(root, &fakeCodebaseIndex{available: true})
	_, err = withIndex.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("search without query err = %v, want invalid_params", err)
	}
}

func TestCodebaseStatusMapsToWire(t *testing.T) {
	root := t.TempDir()
	indexedAt := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	idx := &fakeCodebaseIndex{status: codebaseindex.Status{
		State:      codebaseindex.StateReady,
		ModelID:    "openai:text-embedding-3-small",
		FileCount:  12,
		ChunkCount: 34,
		IndexedAt:  indexedAt,
	}}
	s := serverWithCodebase(root, idx)

	got, err := s.CodebaseStatus(context.Background(), protocol.CodebaseStatusRequest{})
	if err != nil {
		t.Fatalf("codebase status: %v", err)
	}
	if idx.statusRoot != workspacepath.Canonical(root) {
		t.Fatalf("status root = %q, want %q", idx.statusRoot, workspacepath.Canonical(root))
	}
	if got.State != protocol.CodebaseStateReady || got.IndexedAt != indexedAt.Format(time.RFC3339) {
		t.Fatalf("status = %+v", got)
	}
}

func TestCodebaseReindexMapsToWire(t *testing.T) {
	root := t.TempDir()
	idx := &fakeCodebaseIndex{available: true, reindexed: make(chan string, 1)}
	s := serverWithCodebase(root, idx)

	if _, err := s.CodebaseReindex(context.Background(), protocol.CodebaseReindexRequest{}); err != nil {
		t.Fatalf("codebase reindex: %v", err)
	}
	select {
	case got := <-idx.reindexed:
		if got != workspacepath.Canonical(root) {
			t.Fatalf("reindex root = %q, want %q", got, workspacepath.Canonical(root))
		}
	case <-time.After(time.Second):
		t.Fatal("reindex did not start")
	}

	// An unavailable index maps to invalid_params.
	unavailable := serverWithCodebase(root, &fakeCodebaseIndex{available: false})
	if _, err := unavailable.CodebaseReindex(context.Background(), protocol.CodebaseReindexRequest{}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("reindex without embedding err = %v, want invalid_params", err)
	}
}
