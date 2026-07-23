package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitWatcherRejectsUnwatchableGitDirectory(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.Remove(gitDir); err != nil {
		t.Fatalf("remove git dir: %v", err)
	}

	watcher, err := (GitWatcher{}).WatchGitState([]string{root}, func() {})
	if err != nil {
		t.Fatalf("non-repository root should produce an inert watcher: %v", err)
	}
	if err := watcher.Close(); err != nil {
		t.Fatalf("close inert watcher: %v", err)
	}
}
