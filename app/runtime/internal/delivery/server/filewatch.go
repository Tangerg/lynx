package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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
// (the bug this replaces). Agents that can't use platform-specific file
// notification APIs avoid tree-watch the same way: watching only requested
// paths or the .git signal set plus diffs on demand. The agent's OWN edits
// don't need a watcher at all — runsegment publishes their known paths through
// the file-change bridge.
type gitWatcher struct {
	fsw       *fsnotify.Watcher
	emit      func(protocol.WorkspaceEvent)
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
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
		// fixed, tiny FD cost. The resolved .git directory is mandatory: accepting
		// a subscription without it would promise updates this watcher can never
		// deliver. refs/heads is optional until the repository has its first branch.
		if err := fsw.Add(g); err != nil {
			return nil, closeFailedWatcher(fsw, fmt.Errorf("watch git directory %q: %w", g, err))
		}
		refsHeads := filepath.Join(g, "refs", "heads")
		if err := fsw.Add(refsHeads); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, closeFailedWatcher(fsw, fmt.Errorf("watch git refs directory %q: %w", refsHeads, err))
		}
	}
	go w.run()
	return w, nil
}

func closeFailedWatcher(watcher *fsnotify.Watcher, cause error) error {
	if err := watcher.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("close failed git watcher: %w", err))
	}
	return cause
}

func (w *gitWatcher) run() {
	defer close(w.exited)
	timer := time.NewTimer(gitWatchDebounce)
	defer timer.Stop()
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
		case _, ok := <-w.fsw.Errors:
			if !ok {
				// fsnotify closes Events AND Errors together when its read loop
				// dies; without this ok-check a closed Errors channel stays
				// perpetually ready and the select busy-spins. Exit like the
				// Events-closed case.
				return
			}
			// Non-fatal (transient overflow / removed ref dir) — keep watching.
		case <-timer.C:
			armed = false
			w.emit(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventResync})
		}
	}
}

// Close stops the run goroutine and waits for it to exit before closing the
// underlying watcher, so no emit is in flight when the subscription channel is
// closed. Idempotent via sync.Once — a second call is a no-op rather than a
// panic on the already-closed done channel.
func (w *gitWatcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		<-w.exited
		_ = w.fsw.Close()
	})
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
