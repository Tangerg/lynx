package server

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// fileWatchDebounce coalesces a burst of filesystem events into one
// files.changed per watch — an editor write-renames-chmods a file several
// times within a few ms, and the client only needs "something under this watch
// changed → re-fetch" (AUX_API §3.2).
const fileWatchDebounce = 150 * time.Millisecond

// watchTarget is one resolved workspace.subscribe watch: the client-chosen id,
// the cwd emitted paths are relativized against, and the absolute directory
// watched (cwd joined with the jailed relative path).
type watchTarget struct {
	watchID string
	cwdRoot string // abs; emitted paths are relative to this
	absPath string // abs directory watched (cwdRoot + the jailed rel path)
}

// fileWatcher watches the resolved targets and emits a debounced files.changed
// per watch via emit (a lossy send onto the subscription channel). Recursive:
// every directory under a target is added up front and new directories are
// added as they appear (fsnotify itself is non-recursive). Its lifetime is the
// workspace.subscribe request — [fileWatcher.Close] stops the run goroutine
// before the subscription channel is closed, so emit never races that close.
type fileWatcher struct {
	fsw     *fsnotify.Watcher
	targets []watchTarget
	emit    func(protocol.WorkspaceEvent)
	done    chan struct{}
	exited  chan struct{}
}

// startFileWatcher resolves the watch set onto a live recursive fsnotify watch.
func startFileWatcher(targets []watchTarget, emit func(protocol.WorkspaceEvent)) (*fileWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &fileWatcher{
		fsw:     fsw,
		targets: targets,
		emit:    emit,
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
	}
	for _, t := range targets {
		w.addTree(t.absPath)
	}
	go w.run()
	return w, nil
}

// addTree adds root and every existing subdirectory to the watch set —
// fsnotify is non-recursive, so a recursive watch is a walk + per-dir Add.
// Best-effort: unreadable subtrees are skipped, not fatal.
func (w *fileWatcher) addTree(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = w.fsw.Add(path)
		}
		return nil
	})
}

func (w *fileWatcher) run() {
	defer close(w.exited)

	pending := map[string]map[string]struct{}{} // watchID → set of rel paths
	timer := time.NewTimer(fileWatchDebounce)
	timer.Stop() // start idle; armed on the first event
	armed := false

	flush := func() {
		for watchID, paths := range pending {
			w.emit(protocol.WorkspaceEvent{Type: "files.changed", WatchID: watchID, Paths: sortedKeys(paths)})
		}
		pending = map[string]map[string]struct{}{}
		armed = false
	}

	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// A newly-created directory extends the recursive watch.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.fsw.Add(ev.Name)
				}
			}
			t := w.match(ev.Name)
			if t == nil {
				continue
			}
			rel, err := filepath.Rel(t.cwdRoot, ev.Name)
			if err != nil {
				continue
			}
			set := pending[t.watchID]
			if set == nil {
				set = map[string]struct{}{}
				pending[t.watchID] = set
			}
			set[rel] = struct{}{}
			if !armed {
				timer.Reset(fileWatchDebounce)
				armed = true
			}
		case <-w.fsw.Errors:
			// Watcher errors are non-fatal (e.g. a transient overflow) — keep going.
		case <-timer.C:
			flush()
		}
	}
}

// match returns the target whose watched directory contains abs; the longest
// absPath wins when targets nest, so the event is attributed to the most
// specific watch.
func (w *fileWatcher) match(abs string) *watchTarget {
	var best *watchTarget
	for i := range w.targets {
		t := &w.targets[i]
		if abs == t.absPath || strings.HasPrefix(abs, t.absPath+string(filepath.Separator)) {
			if best == nil || len(t.absPath) > len(best.absPath) {
				best = t
			}
		}
	}
	return best
}

// Close stops the run goroutine and waits for it to exit before closing the
// underlying watcher — so no emit is in flight when the caller closes the
// subscription channel.
func (w *fileWatcher) Close() {
	close(w.done)
	<-w.exited
	_ = w.fsw.Close()
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
