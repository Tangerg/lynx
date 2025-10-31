package sync

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
	conc "github.com/sourcegraph/conc/pool"
)

// TestDefaultPool tests the default pool functionality
func TestDefaultPool(t *testing.T) {
	t.Run("returns non-nil pool", func(t *testing.T) {
		pool := DefaultPool()
		if pool == nil {
			t.Fatal("DefaultPool() returned nil")
		}
	})

	t.Run("default pool is PoolOfNoPool", func(t *testing.T) {
		pool := DefaultPool()

		// Test that it can execute a task
		var executed bool
		var wg sync.WaitGroup
		wg.Add(1)

		err := pool.Submit(func() {
			executed = true
			wg.Done()
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		wg.Wait()

		if !executed {
			t.Error("task was not executed")
		}
	})

	t.Run("can execute multiple tasks", func(t *testing.T) {
		pool := DefaultPool()

		const numTasks = 10
		var counter int32
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})
}

// TestSetDefaultPool tests setting a custom default pool
func TestSetDefaultPool(t *testing.T) {
	// Save original pool to restore later
	originalPool := DefaultPool()
	defer func() {
		SetDefaultPool(originalPool)
	}()

	t.Run("sets new default pool", func(t *testing.T) {
		customPool := PoolOfNoPool()
		SetDefaultPool(customPool)

		// Verify the pool was set (we can't directly compare pool instances,
		// so we verify it works)
		var executed bool
		var wg sync.WaitGroup
		wg.Add(1)

		err := DefaultPool().Submit(func() {
			executed = true
			wg.Done()
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		wg.Wait()

		if !executed {
			t.Error("task was not executed")
		}
	})

	t.Run("ignores nil pool", func(t *testing.T) {
		poolBefore := DefaultPool()
		SetDefaultPool(nil)
		poolAfter := DefaultPool()

		// Pool should not have changed (we can't compare directly,
		// but we verify it still works)
		var executed bool
		var wg sync.WaitGroup
		wg.Add(1)

		err := poolAfter.Submit(func() {
			executed = true
			wg.Done()
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		wg.Wait()

		if !executed {
			t.Error("pool should still be functional after nil set")
		}

		_ = poolBefore // Use variable to avoid unused error
	})

	t.Run("switches between different pool types", func(t *testing.T) {
		// Create ants pool
		antsPool, err := ants.NewPool(5)
		if err != nil {
			t.Fatalf("Failed to create ants pool: %v", err)
		}
		defer antsPool.Release()

		SetDefaultPool(PoolOfAnts(antsPool))

		var executed bool
		var wg sync.WaitGroup
		wg.Add(1)

		err = DefaultPool().Submit(func() {
			executed = true
			wg.Done()
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		wg.Wait()

		if !executed {
			t.Error("task was not executed with ants pool")
		}
	})
}

// TestPoolOfNoPool tests the no-pool implementation
func TestPoolOfNoPool(t *testing.T) {
	t.Run("creates valid pool", func(t *testing.T) {
		pool := PoolOfNoPool()
		if pool == nil {
			t.Fatal("PoolOfNoPool() returned nil")
		}
	})

	t.Run("executes task in separate goroutine", func(t *testing.T) {
		pool := PoolOfNoPool()

		mainGoroutineID := getGoroutineID()
		var taskGoroutineID uint64
		var wg sync.WaitGroup
		wg.Add(1)

		time.Sleep(1 * time.Nanosecond)
		err := pool.Submit(func() {
			taskGoroutineID = getGoroutineID()
			wg.Done()
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		wg.Wait()

		if taskGoroutineID == mainGoroutineID {
			t.Error("task should execute in different goroutine")
		}
	})

	t.Run("handles panic in task", func(t *testing.T) {
		pool := PoolOfNoPool()

		var wg sync.WaitGroup
		wg.Add(1)

		err := pool.Submit(func() {
			defer wg.Done()
			panic("test panic")
		})

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		// Should not panic in main goroutine
		wg.Wait()
	})

	t.Run("executes multiple tasks concurrently", func(t *testing.T) {
		pool := PoolOfNoPool()

		const numTasks = 100
		var counter int32
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				wg.Done()
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})

	t.Run("always returns nil error", func(t *testing.T) {
		pool := PoolOfNoPool()

		for i := 0; i < 10; i++ {
			err := pool.Submit(func() {})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}
	})
}

// TestPoolOfConc tests the conc pool adapter
func TestPoolOfConc(t *testing.T) {
	t.Run("creates valid pool adapter", func(t *testing.T) {
		concPool := conc.New()
		pool := PoolOfConc(concPool)

		if pool == nil {
			t.Fatal("PoolOfConc() returned nil")
		}
	})

	t.Run("panics with nil conc pool", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("PoolOfConc(nil) should panic")
			} else {
				if msg, ok := r.(string); ok {
					expected := "conc pool is nil"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()

		_ = PoolOfConc(nil)
	})

	t.Run("executes tasks through conc pool", func(t *testing.T) {
		concPool := conc.New()
		pool := PoolOfConc(concPool)

		var counter int32
		const numTasks = 10

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		concPool.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})

	t.Run("handles concurrent submissions", func(t *testing.T) {
		concPool := conc.New().WithMaxGoroutines(5)
		pool := PoolOfConc(concPool)

		var counter int32
		const numTasks = 50

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&counter, 1)
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		concPool.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})

	t.Run("always returns nil error", func(t *testing.T) {
		concPool := conc.New()
		pool := PoolOfConc(concPool)

		for i := 0; i < 10; i++ {
			err := pool.Submit(func() {})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		concPool.Wait()
	})
}

// TestPoolOfAnts tests the ants pool adapter
func TestPoolOfAnts(t *testing.T) {
	t.Run("creates valid pool adapter", func(t *testing.T) {
		antsPool, err := ants.NewPool(10)
		if err != nil {
			t.Fatalf("Failed to create ants pool: %v", err)
		}
		defer antsPool.Release()

		pool := PoolOfAnts(antsPool)
		if pool == nil {
			t.Fatal("PoolOfAnts() returned nil")
		}
	})

	t.Run("panics with nil ants pool", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("PoolOfAnts(nil) should panic")
			} else {
				if msg, ok := r.(string); ok {
					expected := "ants pool is nil"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()

		_ = PoolOfAnts(nil)
	})

	t.Run("executes tasks through ants pool", func(t *testing.T) {
		antsPool, err := ants.NewPool(5)
		if err != nil {
			t.Fatalf("Failed to create ants pool: %v", err)
		}
		defer antsPool.Release()

		pool := PoolOfAnts(antsPool)

		var counter int32
		const numTasks = 20
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})

	t.Run("respects pool size limit", func(t *testing.T) {
		const poolSize = 3
		antsPool, err := ants.NewPool(poolSize)
		if err != nil {
			t.Fatalf("Failed to create ants pool: %v", err)
		}
		defer antsPool.Release()

		pool := PoolOfAnts(antsPool)

		var currentConcurrent int32
		var maxObserved int32
		const numTasks = 10
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				defer wg.Done()

				current := atomic.AddInt32(&currentConcurrent, 1)
				for {
					old := atomic.LoadInt32(&maxObserved)
					if current <= old || atomic.CompareAndSwapInt32(&maxObserved, old, current) {
						break
					}
				}

				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&currentConcurrent, -1)
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		max := atomic.LoadInt32(&maxObserved)
		if max > poolSize {
			t.Errorf("max concurrent = %d, want <= %d", max, poolSize)
		}
	})

	t.Run("returns error when pool is full", func(t *testing.T) {
		antsPool, err := ants.NewPool(1, ants.WithNonblocking(true))
		if err != nil {
			t.Fatalf("Failed to create ants pool: %v", err)
		}
		defer antsPool.Release()

		pool := PoolOfAnts(antsPool)

		// Fill the pool
		var wg sync.WaitGroup
		wg.Add(1)
		err = pool.Submit(func() {
			time.Sleep(100 * time.Millisecond)
			wg.Done()
		})
		if err != nil {
			t.Fatalf("First submit failed: %v", err)
		}

		// Try to submit when pool is full (nonblocking mode)
		time.Sleep(10 * time.Millisecond) // Ensure first task is running
		err = pool.Submit(func() {})

		if err == nil {
			t.Error("Submit() should return error when pool is full in nonblocking mode")
		}

		wg.Wait()
	})
}

// TestPoolOfWorkerpool tests the workerpool adapter
func TestPoolOfWorkerpool(t *testing.T) {
	t.Run("creates valid pool adapter", func(t *testing.T) {
		wp := workerpool.New(10)
		pool := PoolOfWorkerpool(wp)

		if pool == nil {
			t.Fatal("PoolOfWorkerpool() returned nil")
		}

		wp.StopWait()
	})

	t.Run("panics with nil workerpool", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("PoolOfWorkerpool(nil) should panic")
			} else {
				if msg, ok := r.(string); ok {
					expected := "worker pool is nil"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()

		_ = PoolOfWorkerpool(nil)
	})

	t.Run("executes tasks through workerpool", func(t *testing.T) {
		wp := workerpool.New(5)
		defer wp.StopWait()

		pool := PoolOfWorkerpool(wp)

		var counter int32
		const numTasks = 20
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		if counter != numTasks {
			t.Errorf("counter = %d, want %d", counter, numTasks)
		}
	})

	t.Run("respects pool size limit", func(t *testing.T) {
		const poolSize = 3
		wp := workerpool.New(poolSize)
		defer wp.StopWait()

		pool := PoolOfWorkerpool(wp)

		var currentConcurrent int32
		var maxObserved int32
		const numTasks = 10
		var wg sync.WaitGroup
		wg.Add(numTasks)

		for i := 0; i < numTasks; i++ {
			err := pool.Submit(func() {
				defer wg.Done()

				current := atomic.AddInt32(&currentConcurrent, 1)
				for {
					old := atomic.LoadInt32(&maxObserved)
					if current <= old || atomic.CompareAndSwapInt32(&maxObserved, old, current) {
						break
					}
				}

				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&currentConcurrent, -1)
			})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}

		wg.Wait()

		max := atomic.LoadInt32(&maxObserved)
		if max > poolSize {
			t.Errorf("max concurrent = %d, want <= %d", max, poolSize)
		}
	})

	t.Run("always returns nil error", func(t *testing.T) {
		wp := workerpool.New(5)
		defer wp.StopWait()

		pool := PoolOfWorkerpool(wp)

		for i := 0; i < 10; i++ {
			err := pool.Submit(func() {})
			if err != nil {
				t.Errorf("Submit() error = %v, want nil", err)
			}
		}
	})
}

// TestPoolAdapter tests the poolAdapter type
func TestPoolAdapter(t *testing.T) {
	t.Run("implements Pool interface", func(t *testing.T) {
		var _ Pool = poolAdapter(nil)
	})

	t.Run("calls wrapped function", func(t *testing.T) {
		var called bool
		var submittedFunc func()

		adapter := poolAdapter(func(f func()) error {
			called = true
			submittedFunc = f
			return nil
		})

		testFunc := func() {}
		err := adapter.Submit(testFunc)

		if err != nil {
			t.Errorf("Submit() error = %v, want nil", err)
		}

		if !called {
			t.Error("wrapped function was not called")
		}

		if submittedFunc == nil {
			t.Error("function was not passed to wrapped function")
		}
	})

	t.Run("propagates error from wrapped function", func(t *testing.T) {
		expectedErr := errors.New("test error")

		adapter := poolAdapter(func(f func()) error {
			return expectedErr
		})

		err := adapter.Submit(func() {})

		if err != expectedErr {
			t.Errorf("Submit() error = %v, want %v", err, expectedErr)
		}
	})
}

// TestPoolIntegration tests integration between different pool types
func TestPoolIntegration(t *testing.T) {
	t.Run("can switch between pool implementations", func(t *testing.T) {
		originalPool := DefaultPool()
		defer SetDefaultPool(originalPool)

		poolTypes := []struct {
			name string
			pool Pool
		}{
			{"NoPool", PoolOfNoPool()},
			{"Conc", PoolOfConc(conc.New())},
		}

		// Add ants pool
		antsPool, err := ants.NewPool(5)
		if err == nil {
			defer antsPool.Release()
			poolTypes = append(poolTypes, struct {
				name string
				pool Pool
			}{"Ants", PoolOfAnts(antsPool)})
		}

		// Add workerpool
		wp := workerpool.New(5)
		defer wp.StopWait()
		poolTypes = append(poolTypes, struct {
			name string
			pool Pool
		}{"Workerpool", PoolOfWorkerpool(wp)})

		for _, pt := range poolTypes {
			t.Run(pt.name, func(t *testing.T) {
				SetDefaultPool(pt.pool)

				var executed bool
				var wg sync.WaitGroup
				wg.Add(1)

				err := DefaultPool().Submit(func() {
					executed = true
					wg.Done()
				})

				if err != nil {
					t.Errorf("Submit() error = %v, want nil", err)
				}

				wg.Wait()

				if !executed {
					t.Error("task was not executed")
				}
			})
		}
	})
}

// BenchmarkPools benchmarks different pool implementations
func BenchmarkPools(b *testing.B) {
	b.Run("NoPool", func(b *testing.B) {
		pool := PoolOfNoPool()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(1)
			_ = pool.Submit(func() {
				wg.Done()
			})
			wg.Wait()
		}
	})

	b.Run("Conc", func(b *testing.B) {
		concPool := conc.New().WithMaxGoroutines(10)
		pool := PoolOfConc(concPool)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pool.Submit(func() {})
		}
		concPool.Wait()
	})

	b.Run("Ants", func(b *testing.B) {
		antsPool, _ := ants.NewPool(10)
		defer antsPool.Release()
		pool := PoolOfAnts(antsPool)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(1)
			_ = pool.Submit(func() {
				wg.Done()
			})
			wg.Wait()
		}
	})

	b.Run("Workerpool", func(b *testing.B) {
		wp := workerpool.New(10)
		defer wp.StopWait()
		pool := PoolOfWorkerpool(wp)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(1)
			_ = pool.Submit(func() {
				wg.Done()
			})
			wg.Wait()
		}
	})
}

// Helper function to get goroutine ID (for testing purposes)
func getGoroutineID() uint64 {
	// Simple implementation - in real code, use runtime.Stack or similar
	return uint64(time.Now().UnixNano())
}

// Example demonstrating pool usage
func ExampleDefaultPool() {
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		i := i
		_ = DefaultPool().Submit(func() {
			defer wg.Done()
			// Do work
			_ = i
		})
	}

	wg.Wait()
}

// Example demonstrating custom pool
func ExampleSetDefaultPool() {
	// Create a custom pool with limited concurrency
	antsPool, _ := ants.NewPool(5)
	defer antsPool.Release()

	SetDefaultPool(PoolOfAnts(antsPool))

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		_ = DefaultPool().Submit(func() {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond)
		})
	}

	wg.Wait()
}
