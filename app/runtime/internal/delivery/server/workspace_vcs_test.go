package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestWorkspaceVcsUnavailable(t *testing.T) {
	if !workspace.GitAvailable() {
		t.Skip("git not on PATH")
	}
	s := newWorkspaceServer(t.TempDir())
	if _, err := s.ListWorkspaceFileChanges(context.Background(), protocol.WorkspaceListQuery{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("listFileChanges err = %v, want ErrVcsUnavailable", err)
	}
	if _, err := s.GetWorkspaceDiff(context.Background(), protocol.GetDiffRequest{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("getDiff err = %v, want ErrVcsUnavailable", err)
	}
}

func TestWorkspaceGitWireMapping(t *testing.T) {
	if !workspace.GitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	env := append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitCmd("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", ".")
	gitCmd("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newWorkspaceServer(dir)
	page, err := s.ListWorkspaceFileChanges(context.Background(), protocol.WorkspaceListQuery{})
	if err != nil {
		t.Fatalf("listFileChanges: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].Status != "modified" || page.Data[0].Added == nil {
		t.Fatalf("changes = %+v, want one modified with non-nil added", page.Data)
	}

	diff, err := s.GetWorkspaceDiff(context.Background(), protocol.GetDiffRequest{})
	if err != nil {
		t.Fatalf("getDiff: %v", err)
	}
	if len(diff.Files) != 1 || len(diff.Files[0].Rows) == 0 {
		t.Fatalf("diff = %+v, want one file with rows", diff.Files)
	}
}
