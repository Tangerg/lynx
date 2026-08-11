package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestWorkspaceSubscribe_GitWatch verifies that a real staged index transition
// surfaces a debounced resync without recursively watching the working tree.
func TestWorkspaceSubscribe_GitWatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	fileWatchGitCommand(t, dir, "init", "-q")
	tracked := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("v0\n"), 0o644); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	s := newWorkspaceServer(dir)
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)

	// A git operation semantically changes the staged index → expect a debounced resync.
	if err := os.WriteFile(tracked, []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	select {
	case ev := <-events:
		if ev.Type != "resync" {
			t.Fatalf("event = %+v, want resync", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no resync within 3s of a staged index change")
	}
}

// TestWorkspaceSubscribe_NonRepoInert: a watch on a cwd that isn't a git repo
// contributes no watcher (and doesn't error) — the broadcast stream still works.
func TestWorkspaceSubscribe_NonRepoInert(t *testing.T) {
	dir := t.TempDir() // no .git
	s := newWorkspaceServer(dir)
	s.workspaceHub = newWorkspaceHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicSkillsChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe (non-repo) must not error: %v", err)
	}
	events := drainSeq(ctx, seq)
	// Broadcast events still flow on the subscription.
	s.workspaceHub.publish(protocol.RuntimeEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast event not delivered on a non-repo subscription")
	}
}

func fileWatchGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

// TestRunEffectsNudgePublishesFileChange verifies the application nudge adapter
// reaches the workspace event hub. Tool-item-to-nudge decisions belong to and
// are tested in application/runs.
func TestRunEffectsNudgePublishesFileChange(t *testing.T) {
	s := &Server{workspaceHub: newWorkspaceHub()}
	events, unsub := s.workspaceHub.subscribe()
	defer unsub()

	// Wire the production seam: the run effects publish nudges through the
	// notifier, and the hub observes it (mapping to the wire files.changed).
	fc := &testNotification[workspaceapp.FileChangeNotice]{}
	s.observeFileChanges(fc)
	effects := runsegment.New(runsegment.Config{PublishFileChanges: fc.Publish})

	effects.Nudge("/proj", []string{"src/a.go"})
	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.Workspace == nil || ev.Workspace.Path != "/proj" || len(ev.Paths) != 1 || ev.Paths[0] != "src/a.go" {
			t.Fatalf("event = %+v, want files.changed cwd=/proj [src/a.go]", ev)
		}
	default:
		t.Fatal("write tool call must publish files.changed")
	}

}

// TestWorkspaceSubscribe_MissingWatchID rejects a watch with no id.
func TestWorkspaceSubscribe_MissingWatchID(t *testing.T) {
	s := newWorkspaceServer(t.TempDir())
	s.workspaceHub = newWorkspaceHub()
	if _, _, err := s.SubscribeRuntime(context.Background(), protocol.RuntimeSubscribeRequest{
		Watches: []protocol.WatchSpec{{}},
	}); err == nil {
		t.Fatal("watch missing watchId must be invalid_params")
	}
}
