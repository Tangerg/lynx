package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
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

	watcher, err := NewGitWatcher(t.Context()).Watch([]string{root}, func() {})
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
	watcher, err := NewGitWatcher(t.Context()).Watch([]string{root}, func() { notified <- struct{}{} })
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
	if _, err := ListChanges(t.Context(), root, 10_000); err != nil {
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

func TestGitWatcherResolvesRepositoryFromNestedWorkspace(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	gitCommand(t, root, "init", "-b", "main")
	gitCommand(t, root, "config", "user.name", "t")
	gitCommand(t, root, "config", "user.email", "t@t")
	nested := filepath.Join(root, "packages", "desktop")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested workspace: %v", err)
	}
	tracked := filepath.Join(nested, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitCommand(t, root, "add", "packages/desktop/tracked.txt")
	gitCommand(t, root, "commit", "-m", "initial")

	notified := make(chan struct{}, 1)
	watcher, err := NewGitWatcher(t.Context()).Watch([]string{nested}, func() { notified <- struct{}{} })
	if err != nil {
		t.Fatalf("watch nested workspace: %v", err)
	}
	defer watcher.Close()

	if err := os.WriteFile(tracked, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify nested workspace file: %v", err)
	}
	gitCommand(t, root, "add", "packages/desktop/tracked.txt")
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("nested workspace did not observe its repository index change")
	}
}

func TestGitWatcherKeepsDistinctScopesWithinOneRepository(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	gitCommand(t, root, "init", "-b", "main")
	first := filepath.Join(root, "packages", "first")
	second := filepath.Join(root, "packages", "second")
	for _, workspace := range []string{first, second} {
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatalf("mkdir workspace: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
			t.Fatalf("write tracked file: %v", err)
		}
	}
	gitCommand(t, root, "add", "packages/first/tracked.txt", "packages/second/tracked.txt")

	notified := make(chan struct{}, 1)
	watcher, err := NewGitWatcher(t.Context()).Watch([]string{first, second}, func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch sibling workspace scopes: %v", err)
	}
	defer watcher.Close()

	if err := os.WriteFile(filepath.Join(second, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify second workspace file: %v", err)
	}
	gitCommand(t, root, "add", "packages/second/tracked.txt")
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("second workspace scope did not observe its repository index change")
	}
}

func TestGitWatcherObservesLinkedWorktreeFromNestedWorkspace(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not on PATH")
	}
	base := t.TempDir()
	mainRoot := filepath.Join(base, "main")
	worktreeRoot := filepath.Join(base, "linked")
	if err := os.Mkdir(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main repository: %v", err)
	}
	gitCommand(t, mainRoot, "init", "-b", "main")
	gitCommand(t, mainRoot, "config", "user.name", "t")
	gitCommand(t, mainRoot, "config", "user.email", "t@t")
	mainNested := filepath.Join(mainRoot, "packages", "desktop")
	if err := os.MkdirAll(mainNested, 0o755); err != nil {
		t.Fatalf("mkdir main nested workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainNested, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	gitCommand(t, mainRoot, "add", "packages/desktop/tracked.txt")
	gitCommand(t, mainRoot, "commit", "-m", "initial")
	gitCommand(t, mainRoot, "worktree", "add", "-b", "linked", worktreeRoot)

	nested := filepath.Join(worktreeRoot, "packages", "desktop")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested worktree workspace: %v", err)
	}
	gitDir, commonDir, ok := gitDirectoriesOf(t.Context(), nested)
	if !ok {
		t.Fatal("linked worktree was not resolved as a repository")
	}
	if gitDir == commonDir {
		t.Fatalf("linked worktree git directory %q unexpectedly equals common directory", gitDir)
	}

	notified := make(chan struct{}, 1)
	watcher, err := NewGitWatcher(t.Context()).Watch([]string{nested}, func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("watch linked worktree: %v", err)
	}
	defer watcher.Close()

	tracked := filepath.Join(nested, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify worktree file: %v", err)
	}
	gitCommand(t, worktreeRoot, "add", "packages/desktop/tracked.txt")
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("linked worktree did not observe its private index change")
	}
}

func TestGitWatcherIgnoresAmbientRepositoryRouting(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	foreign := t.TempDir()
	gitCommand(t, root, "init", "-b", "main")
	gitCommand(t, foreign, "init", "-b", "main")

	// Git repository-local variables override -C. Discovery must remain owned
	// by the explicit WorkspaceRef root, or the watcher silently subscribes to
	// another repository's metadata directory.
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)

	gitDir, commonDir, ok := gitDirectoriesOf(t.Context(), root)
	if !ok {
		t.Fatal("requested workspace was not resolved as a repository")
	}
	want, err := pathidentity.Resolve("", filepath.Join(root, ".git"))
	if err != nil {
		t.Fatalf("resolve requested repository identity: %v", err)
	}
	if gitDir != want || commonDir != want {
		t.Fatalf("git directories = (%q, %q), want requested workspace %q", gitDir, commonDir, want)
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
