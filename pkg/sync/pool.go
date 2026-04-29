package sync

import (
	"sync/atomic"

	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
	conc "github.com/sourcegraph/conc/pool"

	"github.com/Tangerg/lynx/pkg/safe"
)

// Pool runs functions concurrently. Implementations may impose limits
// on parallelism, queueing, or per-task timeouts.
type Pool interface {
	Submit(f func()) error
}

// defaultPool is the package-level default Pool, accessed via
// [DefaultPool] / [SetDefaultPool].
var defaultPool atomic.Value

func init() {
	defaultPool.Store(PoolOfNoPool())
}

// DefaultPool returns the current default Pool. Until [SetDefaultPool]
// is called, this is [PoolOfNoPool].
func DefaultPool() Pool {
	return defaultPool.Load().(Pool)
}

// SetDefaultPool replaces the default Pool. A nil pool is ignored.
func SetDefaultPool(p Pool) {
	if p == nil {
		return
	}
	defaultPool.Store(p)
}

// poolAdapter adapts a func(func()) error into the [Pool] interface.
type poolAdapter func(f func()) error

// Submit implements [Pool].
func (p poolAdapter) Submit(f func()) error { return p(f) }

// PoolOfNoPool returns a Pool that launches a fresh recoverable
// goroutine for every task. It applies no concurrency limit.
func PoolOfNoPool() Pool {
	return poolAdapter(func(f func()) error {
		safe.Go(f)
		return nil
	})
}

// PoolOfConc adapts a sourcegraph/conc *Pool. Panics if pool is nil.
func PoolOfConc(pool *conc.Pool) Pool {
	if pool == nil {
		panic("sync: pool must not be nil")
	}
	return poolAdapter(func(f func()) error {
		pool.Go(f)
		return nil
	})
}

// PoolOfAnts adapts a panjf2000/ants *Pool. Panics if pool is nil.
func PoolOfAnts(pool *ants.Pool) Pool {
	if pool == nil {
		panic("sync: pool must not be nil")
	}
	return poolAdapter(func(f func()) error {
		return pool.Submit(f)
	})
}

// PoolOfWorkerpool adapts a gammazero/workerpool *WorkerPool. Panics
// if pool is nil.
func PoolOfWorkerpool(pool *workerpool.WorkerPool) Pool {
	if pool == nil {
		panic("sync: pool must not be nil")
	}
	return poolAdapter(func(f func()) error {
		pool.Submit(f)
		return nil
	})
}
