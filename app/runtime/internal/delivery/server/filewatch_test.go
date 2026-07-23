package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/filechanges"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestWorkspaceSubscribe_GitWatch verifies the cross-platform git-state watch:
// a write to a .git signal file (here .git/index) surfaces a debounced resync.
// No working-tree recursion — so no per-file fd cost on any platform.
func TestWorkspaceSubscribe_GitWatch(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index"), []byte("v0"), 0o644); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	s := &Server{wsHub: newWorkspaceHub(), workspace: newWorkspaceCoordinator(dir, workspaceapp.Config{})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, events, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w1"}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// A git operation rewrites .git/index → expect a debounced resync.
	if err := os.WriteFile(filepath.Join(gitDir, "index"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("touch index: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Type != "resync" {
			t.Fatalf("event = %+v, want resync", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no resync within 3s of a .git change")
	}
}

func TestStartGitWatcherRejectsUnwatchableGitDirectory(t *testing.T) {
	watcher, err := startGitWatcher([]string{filepath.Join(t.TempDir(), "missing", ".git")}, func(protocol.WorkspaceEvent) {})
	if err == nil {
		watcher.Close()
		t.Fatal("startGitWatcher accepted a missing resolved git directory")
	}
}

// TestWorkspaceSubscribe_NonRepoInert: a watch on a cwd that isn't a git repo
// contributes no watcher (and doesn't error) — the broadcast stream still works.
func TestWorkspaceSubscribe_NonRepoInert(t *testing.T) {
	dir := t.TempDir() // no .git
	s := &Server{wsHub: newWorkspaceHub(), workspace: newWorkspaceCoordinator(dir, workspaceapp.Config{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, events, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w1"}},
	})
	if err != nil {
		t.Fatalf("subscribe (non-repo) must not error: %v", err)
	}
	// Broadcast events still flow on the subscription.
	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast event not delivered on a non-repo subscription")
	}
}

// TestRunEffectsNudgePublishesFileChange verifies the application nudge adapter
// reaches the workspace event hub. Tool-item-to-nudge decisions belong to and
// are tested in application/runs.
func TestRunEffectsNudgePublishesFileChange(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}
	events, unsub := s.wsHub.subscribe()
	defer unsub()

	// Wire the production seam: the run effects publish nudges through the
	// notifier, and the hub observes it (mapping to the wire files.changed).
	fc := &filechanges.Notifier{}
	s.wsHub.observe(fc)
	effects := runsegment.New(runsegment.Config{PublishFileChanges: fc.Publish})

	effects.Nudge("/proj", []string{"src/a.go"})
	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.Cwd != "/proj" || len(ev.Paths) != 1 || ev.Paths[0] != "src/a.go" {
			t.Fatalf("event = %+v, want files.changed cwd=/proj [src/a.go]", ev)
		}
	default:
		t.Fatal("write tool call must publish files.changed")
	}

}

// TestWorkspaceSubscribe_MissingWatchID rejects a watch with no id.
func TestWorkspaceSubscribe_MissingWatchID(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub(), workspace: newWorkspaceCoordinator(t.TempDir(), workspaceapp.Config{})}
	if _, _, err := s.WorkspaceSubscribe(context.Background(), protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{}},
	}); err == nil {
		t.Fatal("watch missing watchId must be invalid_params")
	}
}
