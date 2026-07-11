package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// fakeMemoryStore is a knowledge.Store recording the workspace coordinator's
// calls, so the memory delivery handlers can be tested against a wired store
// (or, when nil, against the disabled path).
type fakeMemoryStore struct {
	entries       []knowledge.Entry
	listCwd       string
	getScope      knowledge.Scope
	getCwd        string
	getContent    string
	updateScope   knowledge.Scope
	updateCwd     string
	updateContent string
}

func (s *fakeMemoryStore) List(_ context.Context, cwd string) ([]knowledge.Entry, error) {
	s.listCwd = cwd
	return s.entries, nil
}

func (s *fakeMemoryStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (string, error) {
	s.getScope = scope
	s.getCwd = cwd
	return s.getContent, nil
}

func (s *fakeMemoryStore) Update(_ context.Context, scope knowledge.Scope, cwd string, content string) error {
	s.updateScope = scope
	s.updateCwd = cwd
	s.updateContent = content
	return nil
}

// serverWithMemory builds a test Server whose workspace coordinator is backed by
// store (nil store → the disabled memory path).
func serverWithMemory(store knowledge.Store) *Server {
	s := newTestServer(&stubRuntime{})
	s.workspace = workspaceapp.New(workspaceapp.Config{Memory: store})
	return s
}

func TestListMemoryWithoutStoreReturnsEmptyPage(t *testing.T) {
	s := serverWithMemory(nil)

	got, err := s.ListMemory(context.Background(), protocol.WorkspaceListQuery{
		WorkspaceQuery: protocol.WorkspaceQuery{Cwd: "/repo"},
	})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(got.Data) != 0 {
		t.Fatalf("memory entries = %+v, want empty", got.Data)
	}
}

func TestMemoryHandlersReturnCapabilityErrorWithoutStore(t *testing.T) {
	s := serverWithMemory(nil)

	_, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{Scope: protocol.MemoryScopeHome})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("get memory err = %v, want capability_not_negotiated", err)
	}
	err = s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{Scope: protocol.MemoryScopeHome, Content: "prefs"})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("update memory err = %v, want capability_not_negotiated", err)
	}
}

func TestListMemoryMapsEntriesToWire(t *testing.T) {
	captured := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	repo := t.TempDir()
	store := &fakeMemoryStore{
		entries: []knowledge.Entry{{
			Scope:      knowledge.ScopeUser,
			Content:    "Use short answers",
			CapturedAt: captured,
		}},
	}
	s := serverWithMemory(store)

	got, err := s.ListMemory(context.Background(), protocol.WorkspaceListQuery{
		WorkspaceQuery: protocol.WorkspaceQuery{Cwd: repo},
	})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if store.listCwd != worktree.CanonicalCwd(repo) {
		t.Fatalf("cwd = %q, want %q", store.listCwd, worktree.CanonicalCwd(repo))
	}
	if len(got.Data) != 1 || got.Data[0].Scope != protocol.MemoryScopeHome || got.Data[0].UpdatedAt != captured {
		t.Fatalf("wire memory = %+v", got.Data)
	}
}

func TestGetAndUpdateMemoryMapScopeToRuntime(t *testing.T) {
	store := &fakeMemoryStore{getContent: "project notes"}
	s := serverWithMemory(store)
	repo := t.TempDir()

	got, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{Scope: protocol.MemoryScopeProjectRoot, Cwd: repo})
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Content != "project notes" || store.getScope != knowledge.ScopeProject || store.getCwd != worktree.CanonicalCwd(repo) {
		t.Fatalf("get wire=%+v scope=%v cwd=%q", got, store.getScope, store.getCwd)
	}

	err = s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{
		Scope:   protocol.MemoryScopeHome,
		Cwd:     "/ignored",
		Content: "global prefs",
	})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if store.updateScope != knowledge.ScopeUser || store.updateCwd != "" || store.updateContent != "global prefs" {
		t.Fatalf("update scope=%v cwd=%q content=%q", store.updateScope, store.updateCwd, store.updateContent)
	}
}

func TestProjectMemoryRejectsUnavailableCwd(t *testing.T) {
	store := &fakeMemoryStore{}
	s := serverWithMemory(store)
	missing := filepath.Join(t.TempDir(), "missing")

	if _, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{
		Scope: protocol.MemoryScopeCwd,
		Cwd:   missing,
	}); !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("get memory err = %v, want ErrCwdUnavailable", err)
	}
	if err := s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{
		Scope:   protocol.MemoryScopeProjectRoot,
		Cwd:     missing,
		Content: "notes",
	}); !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("update memory err = %v, want ErrCwdUnavailable", err)
	}
}
