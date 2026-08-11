package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

	watcher, err := (GitWatcher{}).Watch([]string{root}, func() {})
	if err != nil {
		t.Fatalf("non-repository root should produce an inert watcher: %v", err)
	}
	if err := watcher.Close(); err != nil {
		t.Fatalf("close inert watcher: %v", err)
	}
}

func TestGitWatcherIgnoresIndexStatRefreshButPublishesStageChange(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	gitCommand(t, root, "init", "-b", "main")
	gitCommand(t, root, "config", "user.name", "t")
	gitCommand(t, root, "config", "user.email", "t@t")
	tracked := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("stable\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitCommand(t, root, "add", "tracked.txt")
	gitCommand(t, root, "commit", "-m", "initial")

	notified := make(chan struct{}, 2)
	watcher, err := (GitWatcher{}).Watch([]string{root}, func() { notified <- struct{}{} })
	if err != nil {
		t.Fatalf("watch repository: %v", err)
	}
	defer watcher.Close()

	info, err := os.Stat(tracked)
	if err != nil {
		t.Fatalf("stat tracked file: %v", err)
	}
	changed := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(tracked, changed, changed); err != nil {
		t.Fatalf("change tracked timestamp: %v", err)
	}
	// This compound read makes Git refresh index stat data on versions where
	// diff treats that refresh as mandatory. The physical index replacement is
	// not a staged/HEAD change and must stay below the watcher abstraction.
	if _, err := ListChanges(t.Context(), root); err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
	select {
	case <-notified:
		t.Fatal("index stat refresh surfaced as a semantic Git change")
	case <-time.After(2 * gitWatchDebounce):
	}

	if err := os.WriteFile(tracked, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	gitCommand(t, root, "add", "tracked.txt")
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("staged index change was not published")
	}
}

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
