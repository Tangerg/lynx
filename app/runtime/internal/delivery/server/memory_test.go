package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
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

func (r *memoryRuntime) GetMemory(_ context.Context, scope knowledge.Scope, cwd string) (string, error) {
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
	s := &Server{rt: &memoryRuntime{}}

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
	s := &Server{rt: &memoryRuntime{}}

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
	rt := &memoryRuntime{
		enabled: true,
		entries: []knowledge.Entry{{
			Scope:      knowledge.ScopeUser,
			Content:    "Use short answers",
			CapturedAt: captured,
		}},
	}
	s := &Server{rt: rt}

	got, err := s.ListMemory(context.Background(), protocol.WorkspaceListQuery{
		WorkspaceQuery: protocol.WorkspaceQuery{Cwd: "/repo"},
	})
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if rt.listCwd != "/repo" {
		t.Fatalf("cwd = %q, want /repo", rt.listCwd)
	}
	if len(got.Data) != 1 || got.Data[0].Scope != protocol.MemoryScopeHome || got.Data[0].UpdatedAt != captured {
		t.Fatalf("wire memory = %+v", got.Data)
	}
}

func TestGetAndUpdateMemoryMapScopeToRuntime(t *testing.T) {
	rt := &memoryRuntime{enabled: true, getContent: "project notes"}
	s := &Server{rt: rt}

	got, err := s.GetMemory(context.Background(), protocol.GetMemoryRequest{Scope: protocol.MemoryScopeProjectRoot, Cwd: "/repo"})
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Content != "project notes" || rt.getScope != knowledge.ScopeProject || rt.getCwd != "/repo" {
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
	if rt.updateScope != knowledge.ScopeUser || rt.updateCwd != "/ignored" || rt.updateContent != "global prefs" {
		t.Fatalf("update scope=%v cwd=%q content=%q", rt.updateScope, rt.updateCwd, rt.updateContent)
	}
}
