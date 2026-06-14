package server

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
)

// gitWatchDebounce coalesces a burst of .git writes (a single git command
// rewrites several refs/index/HEAD in a few ms) into one resync.
const gitWatchDebounce = 200 * time.Millisecond

// gitWatcher watches a cwd's .git "signal" files — HEAD, index, packed-refs,
// ORIG_HEAD / MERGE_HEAD (direct children of .git) and refs/heads/* — and emits
// a debounced resync when any change, i.e. when git state moves (commit, stage,
// checkout, branch, merge) by ANY process. The client then re-fetches
// workspace.getDiff / listFileChanges.
//
// It deliberately does NOT watch the working tree. On macOS Go's fsnotify uses
// kqueue, which opens a file descriptor per watched file — a recursive watch of
// a real project tree exhausts the process's FDs and takes the server down
// (the bug this replaces). The peers that can't use FSEvents avoid tree-watch
// the same way: codex watches only requested paths, Claude Code watches just
// the .git signal set + diffs on demand. The agent's OWN edits don't need a
// watcher at all — they're emitted as files.changed straight from its
// file-mutating tools (see runs.go emitToolFileChange).
type gitWatcher struct {
	fsw    *fsnotify.Watcher
	emit   func(protocol.WorkspaceEvent)
	done   chan struct{}
	exited chan struct{}
}

// startGitWatcher watches the signal files of each distinct .git directory.
// gitDirs that don't exist (no repo) are skipped — the watch is then inert,
// which is correct (workspace.getDiff would itself report vcs_unavailable).
func startGitWatcher(gitDirs []string, emit func(protocol.WorkspaceEvent)) (*gitWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &gitWatcher{fsw: fsw, emit: emit, done: make(chan struct{}), exited: make(chan struct{})}
	for _, g := range gitDirs {
		// .git holds HEAD / index / packed-refs / ORIG_HEAD / MERGE_HEAD as
		// direct children; refs/heads holds the per-branch refs. Watching these
		// two directories (non-recursive) covers every git state transition at a
		// fixed, tiny FD cost. Best-effort: a missing dir is simply not watched.
		_ = fsw.Add(g)
		_ = fsw.Add(filepath.Join(g, "refs", "heads"))
	}
	go w.run()
	return w, nil
}

func (w *gitWatcher) run() {
	defer close(w.exited)
	timer := time.NewTimer(gitWatchDebounce)
	timer.Stop()
	armed := false
	for {
		select {
		case <-w.done:
			return
		case _, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !armed {
				timer.Reset(gitWatchDebounce)
				armed = true
			}
		case <-w.fsw.Errors:
			// Non-fatal (transient overflow / removed ref dir) — keep watching.
		case <-timer.C:
			armed = false
			w.emit(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventResync})
		}
	}
}

// Close stops the run goroutine and waits for it to exit before closing the
// underlying watcher, so no emit is in flight when the subscription channel is
// closed.
func (w *gitWatcher) Close() {
	close(w.done)
	<-w.exited
	_ = w.fsw.Close()
}

// gitDirOf returns the .git directory for root, or ok=false when root isn't a
// git repository (no .git, or a .git file — worktree/submodule links aren't
// followed in v1; the watch is then inert).
func gitDirOf(root string) (string, bool) {
	g := filepath.Join(root, ".git")
	info, err := os.Stat(g)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return g, true
}
