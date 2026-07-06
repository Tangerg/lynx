package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

type memoryRuntime struct {
	stubRuntime
	enabled       bool
	entries       []knowledge.Entry
	listCwd       string
	getScope      knowledge.Scope
	getCwd        string
	getContent    string
	updateScope   knowledge.Scope
	updateCwd     string
	updateContent string
}

func (r *memoryRuntime) HasMemory() bool {
	return r.enabled
}

func (r *memoryRuntime) ListMemoryEntries(_ context.Context, cwd string) ([]knowledge.Entry, error) {
	r.listCwd = cwd
	return r.entries, nil
}

func (r *memoryRuntime) Memory(_ context.Context, scope knowledge.Scope, cwd string) (string, error) {
	r.getScope = scope
	r.getCwd = cwd
	return r.getContent, nil
}

func (r *memoryRuntime) UpdateMemory(_ context.Context, scope knowledge.Scope, cwd string, content string) error {
	r.updateScope = scope
	r.updateCwd = cwd
	r.updateContent = content
	return nil
}

func TestListMemoryWithoutStoreReturnsEmptyPage(t *testing.T) {
	s := newTestServer(&memoryRuntime{})

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
	s := newTestServer(&memoryRuntime{})

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
	rt := &memoryRuntime{
		enabled: true,
		entries: []knowledge.Entry{{
			Scope:      knowledge.ScopeUser,
			Content:    "Use short answers",
			CapturedAt: captured,
		}},
	}
	s := newTestServer(rt)

	got, err := s.ListMemory(context.Background(), protocol.WorkspaceListQuery{
		WorkspaceQuery: protocol.WorkspaceQuery{Cwd: repo},
	})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if rt.listCwd != worktree.CanonicalCwd(repo) {
		t.Fatalf("cwd = %q, want %q", rt.listCwd, worktree.CanonicalCwd(repo))
	}
	if len(got.Data) != 1 || got.Data[0].Scope != protocol.MemoryScopeHome || got.Data[0].UpdatedAt != captured {
		t.Fatalf("wire memory = %+v", got.Data)
	}
}

func TestGetAndUpdateMemoryMapScopeToRuntime(t *testing.T) {
	rt := &memoryRuntime{enabled: true, getContent: "project notes"}
	s := newTestServer(rt)
	repo := t.TempDir()

	got, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{Scope: protocol.MemoryScopeProjectRoot, Cwd: repo})
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Content != "project notes" || rt.getScope != knowledge.ScopeProject || rt.getCwd != worktree.CanonicalCwd(repo) {
		t.Fatalf("get wire=%+v scope=%v cwd=%q", got, rt.getScope, rt.getCwd)
	}

	err = s.UpdateMemory(context.Background(), protocol.UpdateMemoryRequest{
		Scope:   protocol.MemoryScopeHome,
		Cwd:     "/ignored",
		Content: "global prefs",
	})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if rt.updateScope != knowledge.ScopeUser || rt.updateCwd != "" || rt.updateContent != "global prefs" {
		t.Fatalf("update scope=%v cwd=%q content=%q", rt.updateScope, rt.updateCwd, rt.updateContent)
	}
}

func TestProjectMemoryRejectsUnavailableCwd(t *testing.T) {
	rt := &memoryRuntime{enabled: true}
	s := newTestServer(rt)
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
