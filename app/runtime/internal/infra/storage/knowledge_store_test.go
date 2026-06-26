package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage"
)

func TestFileMemoryService_UpdateAndGet(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())

	svc, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	const userBody = "# User\nprefer terse output\n"
	if err = svc.Update(ctx, knowledge.ScopeUser, "", userBody); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := svc.Get(ctx, knowledge.ScopeUser, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != userBody {
		t.Errorf("Get returned %q, want %q", got, userBody)
	}
}

func TestFileMemoryService_GetEmptyOnFreshHome(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, _ := storage.NewFileKnowledgeStore()
	got, err := svc.Get(context.Background(), knowledge.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("fresh home: want empty, got %q", got)
	}
}

func TestFileMemoryService_PersistsAcrossInstances(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())

	first, _ := storage.NewFileKnowledgeStore()
	_ = first.Update(context.Background(), knowledge.ScopeUser, "", "remember me")

	second, _ := storage.NewFileKnowledgeStore()
	got, _ := second.Get(context.Background(), knowledge.ScopeUser, "")
	if got != "remember me" {
		t.Errorf("after restart got %q", got)
	}
}

func TestFileMemoryService_List_SkipsEmptyScopes(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, _ := storage.NewFileKnowledgeStore()
	ctx := context.Background()

	_ = svc.Update(ctx, knowledge.ScopeUser, "", "only user")

	entries, err := svc.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (project skipped)", len(entries))
	}
	if entries[0].Scope != knowledge.ScopeUser {
		t.Errorf("scope = %d, want user", entries[0].Scope)
	}
	// CapturedAt must be populated from the file mtime, not left zero (the wire
	// maps it to MemoryEntry.UpdatedAt — a zero time would surface as 0001-01-01).
	if entries[0].CapturedAt.IsZero() {
		t.Error("CapturedAt is zero; want the LYRA.md file mtime")
	}
}

// TestFileMemoryService_ProjectScopeUsesCwd points cwd at a temp
// dir and verifies the project file ends up there (not in
// LYRA_HOME).
func TestFileMemoryService_ProjectScopeUsesCwd(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	projectDir := t.TempDir()

	prevWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prevWd) })
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	svc, _ := storage.NewFileKnowledgeStore()
	ctx := context.Background()
	_ = svc.Update(ctx, knowledge.ScopeProject, "", "project body")

	// File should live at <projectDir>/LYRA.md
	body, err := os.ReadFile(filepath.Join(projectDir, "LYRA.md"))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if string(body) != "project body" {
		t.Errorf("project file body = %q", string(body))
	}
}

// TestFileMemoryService_ProjectScopeFollowsDir — the project scope is
// addressed by the per-call dir, so one service serves every project;
// empty dir falls back to the construction-time default.
func TestFileMemoryService_ProjectScopeFollowsDir(t *testing.T) {
	t.Setenv("LYRA_HOME", t.TempDir())
	svc, err := storage.NewFileKnowledgeStore()
	if err != nil {
		t.Fatalf("NewFileKnowledgeStore: %v", err)
	}

	dirA, dirB := t.TempDir(), t.TempDir()
	ctx := context.Background()
	if err := svc.Update(ctx, knowledge.ScopeProject, dirA, "alpha knowledge"); err != nil {
		t.Fatalf("Update dirA: %v", err)
	}

	got, err := svc.Get(ctx, knowledge.ScopeProject, dirA)
	if err != nil || got != "alpha knowledge" {
		t.Fatalf("Get dirA = (%q, %v), want alpha knowledge", got, err)
	}
	if got, _ := svc.Get(ctx, knowledge.ScopeProject, dirB); got != "" {
		t.Fatalf("Get dirB = %q, want empty (projects are isolated)", got)
	}
}
