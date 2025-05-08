// Package future provides a unified interface for various goroutine pool implementations.
// It allows interchangeable usage of different concurrency libraries through a common API.
package future

import (
	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
	conc "github.com/sourcegraph/conc/pool"
	"sync/atomic"
)

// Pool defines the common interface for all goroutine pool implementations.
// Any pool implementing this interface can be used to execute functions concurrently.
type Pool interface {
	// Go submits a function to be executed concurrently by the pool.
	Go(f func())
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
	defaultPool.Store(PoolOfGoroutines())
}

// poolWrapper is an adapter type that converts a function with the signature
// func(func()) into a Pool implementation.
type poolWrapper func(f func())

// Go implements the Pool interface for poolWrapper by calling the wrapped function.
func (p poolWrapper) Go(f func()) {
	p(f)
}

// PoolOfGoroutines creates a Pool that simply launches a new goroutine for each task.
// This implementation has no limits on concurrency and doesn't provide any pooling benefits.
// It does include basic panic recovery for safety.
func PoolOfGoroutines() Pool {
	return poolWrapper(func(f func()) {
		go func() {
			defer func() {
				recover()
			}()
			f()
		}()
	})
}

// PoolOfConc creates a Pool adapter for the sourcegraph/conc pool implementation.
// It panics if the provided pool is nil.
func PoolOfConc(pool *conc.Pool) Pool {
	if pool == nil {
		panic("conc pool is nil")
	}
	return poolWrapper(func(f func()) {
		pool.Go(f)
	})
}

// PoolOfAnts creates a Pool adapter for the panjf2000/ants pool implementation.
// It panics if the provided pool is nil.
func PoolOfAnts(pool *ants.Pool) Pool {
	if pool == nil {
		panic("ants pool is nil")
	}
	return poolWrapper(func(f func()) {
		_ = pool.Submit(f)
	})
}

// PoolOfWorkerpool creates a Pool adapter for the gammazero/workerpool implementation.
// It panics if the provided pool is nil.
func PoolOfWorkerpool(pool *workerpool.WorkerPool) Pool {
	if pool == nil {
		panic("worker pool is nil")
	}
	return poolWrapper(func(f func()) {
		pool.Submit(f)
	})
}
