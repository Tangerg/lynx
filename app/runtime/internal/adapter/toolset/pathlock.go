package toolset

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/tools"
)

// pathLocker serializes file tool calls that target the same resolved path.
// Separate runs can execute concurrently, so two write/edit calls must not
// interleave, and a tracked read must stamp the exact state it read.
// Keyed by resolved abs path and ref-counted so the map doesn't grow unbounded;
// glob / grep / LSP / MCP tools are unaffected. The runtime resolver owns one
// locker across all of its per-turn tool builds, so separate turns cannot both
// cross a stale-read check before either mutation lands.
type pathLocker struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

type pathLock struct {
	ready chan struct{}
	refs  int
}

func newPathLocker() *pathLocker { return &pathLocker{locks: make(map[string]*pathLock)} }

// acquire takes the per-path lock and returns a release closure. Calls for
// distinct paths run concurrently; calls for the same path serialize. Waiting
// is context-aware so a canceled turn is never pinned behind another tool call.
func (p *pathLocker) acquire(ctx context.Context, path string) (func(), error) {
	p.mu.Lock()
	l := p.locks[path]
	if l == nil {
		l = &pathLock{ready: make(chan struct{}, 1)}
		l.ready <- struct{}{}
		p.locks[path] = l
	}
	l.refs++
	p.mu.Unlock()

	if err := ctx.Err(); err != nil {
		p.releaseRef(path, l)
		return nil, err
	}
	select {
	case <-ctx.Done():
		p.releaseRef(path, l)
		return nil, ctx.Err()
	case <-l.ready:
	}
	if err := ctx.Err(); err != nil {
		l.ready <- struct{}{}
		p.releaseRef(path, l)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			l.ready <- struct{}{}
			p.releaseRef(path, l)
		})
	}, nil
}

func (p *pathLocker) releaseRef(path string, l *pathLock) {
	p.mu.Lock()
	if l.refs--; l.refs == 0 {
		delete(p.locks, path)
	}
	p.mu.Unlock()
}

// withPathLock wraps a file tool so concurrent calls targeting the same resolved
// path run one-at-a-time (see [pathLocker]). For mutations it is applied inside
// the path guard but outside the staleness / diagnostics / mutation chain. For
// reads it encloses both the filesystem read and tracker stamp.
func withPathLock(inner tools.Tool, locker *pathLocker, workdir string) tools.Tool {
	if locker == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := resolvedMutatedPaths(inner, arguments, workdir)
		if len(paths) == 0 {
			return inner.Call(ctx, arguments)
		}
		releases := make([]func(), 0, len(paths))
		for _, path := range paths {
			release, err := locker.acquire(ctx, path)
			if err != nil {
				for i := len(releases) - 1; i >= 0; i-- {
					releases[i]()
				}
				return "", err
			}
			releases = append(releases, release)
		}
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()
		return inner.Call(ctx, arguments)
	})
}
