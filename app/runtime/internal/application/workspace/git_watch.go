package workspace

import (
	"errors"
	"io"
)

// ErrFileWatchUnavailable reports that Git-state observation is unavailable.
var ErrFileWatchUnavailable = errors.New("workspace: file watch unavailable")

// GitStateWatcher owns filesystem notification, debounce, repository layout,
// and goroutine lifetime for Git metadata changes.
type GitStateWatcher interface {
	Watch(roots []string, notify func()) (io.Closer, error)
}

// GitWatch resolves requested workspaces before delegating technical watching.
type GitWatch struct {
	scope   *Scope
	watcher GitStateWatcher
}

func NewGitWatch(scope *Scope, watcher GitStateWatcher) *GitWatch {
	return &GitWatch{scope: scope, watcher: watcher}
}

// Available reports whether Git-state observation is wired.
func (g *GitWatch) Available() bool { return g != nil && g.watcher != nil }

// Watch canonicalizes and deduplicates workspace roots before watching.
func (g *GitWatch) Watch(cwds []string, notify func()) (io.Closer, error) {
	if g.watcher == nil {
		return nil, ErrFileWatchUnavailable
	}
	seen := make(map[string]struct{}, len(cwds))
	roots := make([]string, 0, len(cwds))
	for _, cwd := range cwds {
		root, err := g.scope.root(cwd)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return g.watcher.Watch(roots, notify)
}
