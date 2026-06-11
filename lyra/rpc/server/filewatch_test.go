package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestWorkspaceSubscribe_FileWatch verifies the watches path (AUX_API §3):
// a write under a watched directory surfaces a debounced files.changed naming
// the changed path (relative to the watch cwd) and echoing the client watchId.
func TestWorkspaceSubscribe_FileWatch(t *testing.T) {
	dir := t.TempDir()
	s := &Server{wsHub: newWorkspaceHub(), serverInfo: protocol.ServerInfo{Cwd: dir}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, events, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w1"}}, // empty path → watch the cwd root
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.WatchID != "w1" {
			t.Fatalf("event = %+v, want files.changed for w1", ev)
		}
		found := false
		for _, p := range ev.Paths {
			if p == "a.txt" {
				found = true
			}
		}
		if !found {
			t.Fatalf("paths = %v, want to include a.txt", ev.Paths)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no files.changed within 3s")
	}
}

// TestFileWatch_IgnoresVCSDir verifies the watch skips dependency / VCS dirs
// (here .git) — they dominate a tree's FD cost on macOS (kqueue = 1 fd/file)
// and are never what a client wants change events for. A change inside .git
// surfaces nothing; a change at the root still does.
func TestFileWatch_IgnoresVCSDir(t *testing.T) {
	dir := t.TempDir()
	// Pre-create .git/config so subscribing doesn't see a mkdir event for it.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed .git/config: %v", err)
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

	// Touch an ignored path and a watched one; only the watched one should surface.
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("changed"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)

	select {
	case ev := <-events:
		for _, p := range ev.Paths {
			if strings.HasPrefix(p, ".git") {
				t.Fatalf("paths = %v, must not include the ignored .git subtree", ev.Paths)
			}
		}
		if !slices.Contains(ev.Paths, "a.txt") {
			t.Fatalf("paths = %v, want a.txt", ev.Paths)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no files.changed within 3s")
	}
}

// TestWorkspaceSubscribe_WatchJail rejects a watch escaping its cwd
// (path_outside_root) and a missing / non-directory target (invalid_params),
// rather than silently dropping the watch.
func TestWorkspaceSubscribe_WatchJail(t *testing.T) {
	dir := t.TempDir()
	s := &Server{wsHub: newWorkspaceHub(), serverInfo: protocol.ServerInfo{Cwd: dir}}
	ctx := context.Background()

	if _, _, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w1", Path: "../escape"}},
	}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Fatalf("escaping watch err = %v, want ErrPathOutsideRoot", err)
	}

	if _, _, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w1", Path: "nope.txt"}},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("missing-dir watch err = %v, want ErrInvalidParams", err)
	}

	if _, _, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{Path: ""}}, // no watchId
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("missing watchId err = %v, want ErrInvalidParams", err)
	}
}
