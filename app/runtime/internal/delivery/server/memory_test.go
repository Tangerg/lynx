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
)

// fakeMemoryStore is a workspace knowledge store recording the coordinator's
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
func serverWithMemory(store workspaceapp.KnowledgeStore) *Server {
	s := newTestServer(&stubRuntime{})
	applyWorkspaceSurfaces(s, newWorkspaceSurfaces("", workspaceTestConfig{Memory: store}))
	s.features.memory = store != nil
	return s
}

func TestListMemoryWithoutStoreReturnsCapabilityError(t *testing.T) {
	s := serverWithMemory(nil)

	_, err := s.ListMemory(context.Background(), protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: "/repo"},
	})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list memory err = %v, want capability_not_negotiated", err)
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

	got, err := s.ListMemory(context.Background(), protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: repo},
	})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if store.listCwd != canonicalWorkspacePath(t, repo) {
		t.Fatalf("cwd = %q, want %q", store.listCwd, canonicalWorkspacePath(t, repo))
	}
	if len(got.Data) != 1 || got.Data[0].Scope != protocol.MemoryScopeHome || got.Data[0].UpdatedAt != captured {
		t.Fatalf("wire memory = %+v", got.Data)
	}
}

func TestGetAndUpdateMemoryMapScopeToRuntime(t *testing.T) {
	store := &fakeMemoryStore{getContent: "project notes"}
	s := serverWithMemory(store)
	repo := t.TempDir()

	got, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{
		Scope: protocol.MemoryScopeProjectRoot, Workspace: &protocol.WorkspaceRef{Path: repo},
	})
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Content != "project notes" || store.getScope != knowledge.ScopeProject || store.getCwd != canonicalWorkspacePath(t, repo) {
		t.Fatalf("get wire=%+v scope=%v cwd=%q", got, store.getScope, store.getCwd)
	}

	err = s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{
		Scope:     protocol.MemoryScopeHome,
		Workspace: &protocol.WorkspaceRef{Path: "/ignored"},
		Content:   "global prefs",
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
		Scope:     protocol.MemoryScopeCwd,
		Workspace: &protocol.WorkspaceRef{Path: missing},
	}); !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("get memory err = %v, want ErrWorkspaceUnavailable", err)
	}
	if err := s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{
		Scope:     protocol.MemoryScopeProjectRoot,
		Workspace: &protocol.WorkspaceRef{Path: missing},
		Content:   "notes",
	}); !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("update memory err = %v, want ErrWorkspaceUnavailable", err)
	}
}

func TestMemoryMappingRejectsUnknownScopes(t *testing.T) {
	s := serverWithMemory(&fakeMemoryStore{})

	if _, err := s.GetMemory(t.Context(), protocol.GetMemoryRequest{
		Scope: protocol.MemoryScope("workspace"),
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("GetMemory err = %v, want ErrInvalidParams", err)
	}
	if _, err := presentMemoryScope(knowledge.Scope("workspace")); err == nil {
		t.Fatal("presentMemoryScope accepted an unknown domain scope")
	}
}
