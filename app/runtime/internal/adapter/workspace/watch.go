package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

// GitWatcher adapts platform filesystem notifications to the workspace
// application's Git-state observation port. It watches only .git signal
// directories (never a project tree), keeping the descriptor cost fixed even
// for large repositories.
type GitWatcher struct{}

var _ workspaceapp.GitStateWatcher = GitWatcher{}

const gitWatchDebounce = 200 * time.Millisecond

// WatchGitState watches every distinct repository reached from roots. A
// non-repository root is intentionally inert: its diff view is unavailable as
// well, but the surrounding workspace subscription remains valid.
func (GitWatcher) WatchGitState(roots []string, notify func()) (io.Closer, error) {
	gitDirs := gitDirsForRoots(roots)
	if len(gitDirs) == 0 {
		return nopWatch{}, nil
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &gitWatch{fsw: fsw, notify: notify, done: make(chan struct{}), exited: make(chan struct{})}
	for _, gitDir := range gitDirs {
		// .git directly holds HEAD/index/packed-refs and refs/heads holds branch
		// tips. Both are non-recursive, bounded watches; refs/heads may not exist
		// until a repository has its first branch.
		if err := fsw.Add(gitDir); err != nil {
			return nil, closeFailedWatch(fsw, fmt.Errorf("watch git directory %q: %w", gitDir, err))
		}
		refsHeads := filepath.Join(gitDir, "refs", "heads")
		if err := fsw.Add(refsHeads); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, closeFailedWatch(fsw, fmt.Errorf("watch git refs directory %q: %w", refsHeads, err))
		}
	}
	go w.run()
	return w, nil
}

func gitDirsForRoots(roots []string) []string {
	seen := make(map[string]struct{}, len(roots))
	gitDirs := make([]string, 0, len(roots))
	for _, root := range roots {
		gitDir, ok := gitDirOf(root)
		if !ok {
			continue
		}
		if _, duplicate := seen[gitDir]; duplicate {
			continue
		}
		seen[gitDir] = struct{}{}
		gitDirs = append(gitDirs, gitDir)
	}
	return gitDirs
}

func gitDirOf(root string) (string, bool) {
	gitDir := filepath.Join(root, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return gitDir, true
}

type gitWatch struct {
	fsw       *fsnotify.Watcher
	notify    func()
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
}

func (w *gitWatch) run() {
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
				return
			}
			// A transient overflow or removed ref directory does not invalidate the
			// subscription. The client will re-fetch on the next resync.
		case <-timer.C:
			armed = false
			if w.notify != nil {
				w.notify()
			}
		}
	}
}

// Close joins the callback goroutine before closing the underlying watcher, so
// a caller can safely close its delivery channel immediately afterwards.
func (w *gitWatch) Close() error {
	w.closeOnce.Do(func() {
		close(w.done)
		<-w.exited
		_ = w.fsw.Close()
	})
	return nil
}

func closeFailedWatch(watcher *fsnotify.Watcher, cause error) error {
	if err := watcher.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("close failed git watcher: %w", err))
	}
	return cause
}

type nopWatch struct{}

func (nopWatch) Close() error { return nil }
