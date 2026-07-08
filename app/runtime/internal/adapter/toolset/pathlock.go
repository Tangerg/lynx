package toolset

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

// pathLocker serializes file-mutating tool calls that target the same resolved
// path. The agent runs tool_calls in parallel (ParallelToolLoop), so without
// this two concurrent write/edit calls to one file could interleave and corrupt
// it. Keyed by resolved abs path, ref-counted so the map doesn't grow unbounded;
// read / glob / grep / LSP / MCP tools are unaffected (they don't acquire). One
// locker is shared by a build's write + edit tools (see BuildWorkdirTools).
type pathLocker struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

type pathLock struct {
	mu   sync.Mutex
	refs int
}

func newPathLocker() *pathLocker { return &pathLocker{locks: make(map[string]*pathLock)} }

// acquire takes the per-path lock and returns a release closure. Calls for
// distinct paths run concurrently; calls for the same path serialize.
func (p *pathLocker) acquire(path string) func() {
	p.mu.Lock()
	l := p.locks[path]
	if l == nil {
		l = &pathLock{}
		p.locks[path] = l
	}
	l.refs++
	p.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		p.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(p.locks, path)
		}
		p.mu.Unlock()
	}
}

// withPathLock wraps a file-mutating tool so concurrent calls targeting the same
// resolved path run one-at-a-time (see [pathLocker]). Applied INSIDE the path
// guard (no lock for a refused write) but OUTSIDE the staleness / diagnostics /
// mutation chain, so the read-before check and the write stay atomic per path.
func withPathLock(inner chat.Tool, locker *pathLocker, workdir string) chat.Tool {
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
			releases = append(releases, locker.acquire(path))
		}
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()
		return inner.Call(ctx, arguments)
	})
}
