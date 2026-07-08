package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

type codebaseRuntime struct {
	stubRuntime
	enabled     bool
	hits        []codebaseindex.Hit
	status      codebaseindex.Status
	searchRoot  string
	searchQuery string
	searchLimit int
	statusRoot  string
	reindexRoot string
	reindexErr  error
}

func (r *codebaseRuntime) HasCodebaseIndex() bool {
	return r.enabled
}

func (r *codebaseRuntime) SearchCodebase(_ context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	r.searchRoot = root
	r.searchQuery = query
	r.searchLimit = limit
	return r.hits, nil
}

func (r *codebaseRuntime) CodebaseIndexStatus(_ context.Context, root string) (codebaseindex.Status, error) {
	r.statusRoot = root
	return r.status, nil
}

func (r *codebaseRuntime) StartCodebaseReindex(_ context.Context, root string) error {
	r.reindexRoot = root
	return r.reindexErr
}

func TestCodebaseSearchUsesRuntimeFacade(t *testing.T) {
	root := t.TempDir()
	rt := &codebaseRuntime{
		enabled: true,
		hits: []codebaseindex.Hit{{
			Path:      "runtime/session.go",
			StartLine: 10,
			EndLine:   12,
			Text:      "type Session struct{}",
			Score:     0.9,
		}},
	}
	s := newTestServerWithInfo(rt, protocol.ServerInfo{Cwd: root})

	got, err := s.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{Query: "session", Limit: 3})
	if err != nil {
		t.Fatalf("codebase search: %v", err)
	}
	if rt.searchRoot != worktree.CanonicalCwd(root) || rt.searchQuery != "session" || rt.searchLimit != 3 {
		t.Fatalf("search root=%q query=%q limit=%d", rt.searchRoot, rt.searchQuery, rt.searchLimit)
	}
	if len(got.Hits) != 1 || got.Hits[0].Path != "runtime/session.go" || got.Hits[0].Score != 0.9 {
		t.Fatalf("wire hits = %+v", got.Hits)
	}
}

func TestCodebaseSearchRequiresIndexAndQuery(t *testing.T) {
	root := t.TempDir()
	s := newTestServerWithInfo(&codebaseRuntime{}, protocol.ServerInfo{Cwd: root})

	_, err := s.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{Query: "session"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("search without index err = %v, want invalid_params", err)
	}

	s.rt = &codebaseRuntime{enabled: true}
	_, err = s.CodebaseSearch(context.Background(), protocol.CodebaseSearchRequest{})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("search without query err = %v, want invalid_params", err)
	}
}

func TestCodebaseStatusUsesRuntimeFacade(t *testing.T) {
	root := t.TempDir()
	indexedAt := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	rt := &codebaseRuntime{status: codebaseindex.Status{
		State:      codebaseindex.StateReady,
		ModelID:    "openai:text-embedding-3-small",
		FileCount:  12,
		ChunkCount: 34,
		IndexedAt:  indexedAt,
	}}
	s := newTestServerWithInfo(rt, protocol.ServerInfo{Cwd: root})

	got, err := s.CodebaseStatus(context.Background(), protocol.CodebaseStatusRequest{})
	if err != nil {
		t.Fatalf("codebase status: %v", err)
	}
	if rt.statusRoot != worktree.CanonicalCwd(root) {
		t.Fatalf("status root = %q, want %q", rt.statusRoot, worktree.CanonicalCwd(root))
	}
	if got.State != protocol.CodebaseStateReady || got.IndexedAt != indexedAt.Format(time.RFC3339) {
		t.Fatalf("status = %+v", got)
	}
}

func TestCodebaseReindexUsesRuntimeFacade(t *testing.T) {
	root := t.TempDir()
	rt := &codebaseRuntime{}
	s := newTestServerWithInfo(rt, protocol.ServerInfo{Cwd: root})

	if err := s.CodebaseReindex(context.Background(), protocol.CodebaseReindexRequest{}); err != nil {
		t.Fatalf("codebase reindex: %v", err)
	}
	if rt.reindexRoot != worktree.CanonicalCwd(root) {
		t.Fatalf("reindex root = %q, want %q", rt.reindexRoot, worktree.CanonicalCwd(root))
	}

	rt.reindexErr = codebaseindex.ErrNoEmbeddingModel
	if err := s.CodebaseReindex(context.Background(), protocol.CodebaseReindexRequest{}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("reindex without embedding err = %v, want invalid_params", err)
	}
}
