package toolset

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

const fileResourceKeyPrefix = "file:"

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
// reads it encloses both the filesystem read and tracker stamp. The wrapper also
// replaces a concurrency-safe tool's caller-spelled path key with the same
// canonical physical identity used by the lock. This makes model-order
// scheduling agree with execution for relative, absolute, and symlink aliases.
func withPathLock(inner tools.Tool, locker *pathLocker, workdir string) tools.Tool {
	if locker == nil {
		return inner
	}
	return &pathLockedTool{inner: inner, locker: locker, workdir: workdir}
}

// pathLockedTool owns the full same-file execution contract: its scheduling
// key and runtime lock are derived from the same canonical path function.
type pathLockedTool struct {
	inner   tools.Tool
	locker  *pathLocker
	workdir string
}

func (t *pathLockedTool) Definition() chat.ToolDefinition { return t.inner.Definition() }

func (t *pathLockedTool) Call(ctx context.Context, arguments string) (string, error) {
	for _, path := range resolvedMutationPaths(t.inner, arguments, t.workdir) {
		release, err := t.locker.acquire(ctx, path)
		if err != nil {
			return "", err
		}
		defer release()
	}
	return t.inner.Call(ctx, arguments)
}

func (t *pathLockedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	capability, ok := t.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	})
	if !ok {
		return "", false
	}
	key, concurrent = capability.ConcurrencyKey(arguments)
	if !concurrent {
		return "", false
	}
	paths := resolvedMutationPaths(t.inner, arguments, t.workdir)
	switch len(paths) {
	case 0:
		return key, true
	case 1:
		return fileResourceKeyPrefix + paths[0], true
	default:
		// One key cannot express partial overlap between multi-file calls.
		// Keep such a tool exclusive until the scheduler has a resource-set
		// contract.
		return "", false
	}
}

func (t *pathLockedTool) ReturnsDirect() bool {
	if direct, ok := t.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

func (t *pathLockedTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := t.inner.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}
