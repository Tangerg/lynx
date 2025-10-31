package sync

import (
	"sync/atomic"

	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
	conc "github.com/sourcegraph/conc/pool"

	"github.com/Tangerg/lynx/pkg/safe"
)

// Pool defines the common interface for all goroutine pool implementations.
// Any pool implementing this interface can be used to execute functions concurrently.
type Pool interface {
	// Submit submits a function to be executed concurrently by the pool.
	Submit(f func()) error
}

// defaultPool is the package-level default pool instance.
// It uses a simple goroutine-based implementation with no limits.
var defaultPool atomic.Value

// DefaultPool returns the current default pool instance.
func DefaultPool() Pool {
	return defaultPool.Load().(Pool)
}

// SetDefaultPool sets a new default pool for the package.
// If the provided pool is nil, the function has no effect.
func SetDefaultPool(pool Pool) {
	if pool == nil {
		return
	}
	defaultPool.Store(pool)
}

// init initializes the package by setting the default pool to a simple goroutine pool.
func init() {
	defaultPool.Store(PoolOfNoPool())
}

// poolAdapter is an adapter type that converts a function with the signature
// func(func()) into a Pool implementation.
type poolAdapter func(f func()) error

// Submit implements the Pool interface for poolAdapter by calling the wrapped function.
func (p poolAdapter) Submit(f func()) error {
	return p(f)
}

// PoolOfNoPool creates a Pool that simply launches a new goroutine for each task.
// This implementation has no limits on concurrency and doesn't provide any pooling benefits.
// It does include basic panic recovery for safety by Go.
func PoolOfNoPool() Pool {
	return poolAdapter(func(f func()) error {
		safe.Go(f)
		return nil
	})
}

// PoolOfConc creates a Pool adapter for the sourcegraph/conc pool implementation.
// It panics if the provided pool is nil.
func PoolOfConc(pool *conc.Pool) Pool {
	if pool == nil {
		panic("conc pool is nil")
	}
	return poolAdapter(func(f func()) error {
		pool.Go(f)
		return nil
	})
}

// PoolOfAnts creates a Pool adapter for the panjf2000/ants pool implementation.
// It panics if the provided pool is nil.
func PoolOfAnts(pool *ants.Pool) Pool {
	if pool == nil {
		panic("ants pool is nil")
	}
	return poolAdapter(func(f func()) error {
		return pool.Submit(f)
	})
}

// PoolOfWorkerpool creates a Pool adapter for the gammazero/workerpool implementation.
// It panics if the provided pool is nil.
func PoolOfWorkerpool(pool *workerpool.WorkerPool) Pool {
	if pool == nil {
		panic("worker pool is nil")
	}
	return poolAdapter(func(f func()) error {
		pool.Submit(f)
		return nil
	})
}
