package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestWorkspaceSubscribe_GitWatch verifies the cross-platform git-state watch:
// a write to a .git signal file (here .git/index) surfaces a debounced resync.
// No working-tree recursion — so no per-file fd cost on any platform.
func TestWorkspaceSubscribe_GitWatch(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index"), []byte("v0"), 0o644); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	s := &Server{wsHub: newWorkspaceHub(), serverInfo: protocol.ServerInfo{Cwd: dir}}

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

// TestWorkspaceSubscribe_NonRepoInert: a watch on a cwd that isn't a git repo
// contributes no watcher (and doesn't error) — the broadcast stream still works.
func TestWorkspaceSubscribe_NonRepoInert(t *testing.T) {
	dir := t.TempDir() // no .git
	s := &Server{wsHub: newWorkspaceHub(), serverInfo: protocol.ServerInfo{Cwd: dir}}
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

// TestEmitToolFileChange: a completed write/edit tool call publishes a
// files.changed naming the exact (cwd-relative) path; bash, errored, and
// non-tool items publish nothing.
func TestEmitToolFileChange(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}
	events, unsub := s.wsHub.subscribe()
	defer unsub()

	completed := func(name, path string, failed bool) protocol.StreamEvent {
		it := &protocol.Item{
			Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
			Tool: &protocol.ToolInvocation{Name: name, Arguments: map[string]any{"file_path": path}},
		}
		if failed {
			it.Error = &protocol.ProblemData{Type: "tool_failed"}
		}
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: it}
	}

	// write → files.changed{cwd, [path]}
	s.emitToolFileChange("/proj", completed("write", "src/a.go", false))
	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.Cwd != "/proj" || len(ev.Paths) != 1 || ev.Paths[0] != "src/a.go" {
			t.Fatalf("event = %+v, want files.changed cwd=/proj [src/a.go]", ev)
		}
	default:
		t.Fatal("write tool call must publish files.changed")
	}

	// bash, errored write, and a non-tool item → nothing.
	s.emitToolFileChange("/proj", completed("bash", "whatever", false))
	s.emitToolFileChange("/proj", completed("write", "src/b.go", true))
	s.emitToolFileChange("/proj", protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &protocol.Item{Type: protocol.ItemTypeAgentMessage}})
	select {
	case ev := <-events:
		t.Fatalf("expected no further events, got %+v", ev)
	default:
	}
}

// TestWorkspaceSubscribe_MissingWatchID rejects a watch with no id.
func TestWorkspaceSubscribe_MissingWatchID(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub(), serverInfo: protocol.ServerInfo{Cwd: t.TempDir()}}
	if _, _, err := s.WorkspaceSubscribe(context.Background(), protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{}},
	}); err == nil {
		t.Fatal("watch missing watchId must be invalid_params")
	}
}
