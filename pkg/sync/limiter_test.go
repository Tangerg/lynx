package sync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewLimiter tests the NewLimiter constructor
func TestNewLimiter(t *testing.T) {
	t.Run("creates limiter with valid max", func(t *testing.T) {
		limiter := NewLimiter(5)
		if limiter == nil {
			t.Fatal("NewLimiter returned nil")
		}
		if limiter.semaphore == nil {
			t.Fatal("semaphore channel is nil")
		}
		if cap(limiter.semaphore) != 5 {
			t.Errorf("semaphore capacity = %d, want 5", cap(limiter.semaphore))
		}
	})

	t.Run("creates limiter with max 1", func(t *testing.T) {
		limiter := NewLimiter(1)
		if cap(limiter.semaphore) != 1 {
			t.Errorf("semaphore capacity = %d, want 1", cap(limiter.semaphore))
		}
	})

	t.Run("creates limiter with large max", func(t *testing.T) {
		limiter := NewLimiter(10000)
		if cap(limiter.semaphore) != 10000 {
			t.Errorf("semaphore capacity = %d, want 10000", cap(limiter.semaphore))
		}
	})

	t.Run("panics with zero max", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("NewLimiter(0) should panic")
			} else {
				if msg, ok := r.(string); ok {
					expected := "max must be > 0"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()
		_ = NewLimiter(0)
	})

	t.Run("panics with negative max", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("NewLimiter(-1) should panic")
			}
		}()
		_ = NewLimiter(-1)
	})
}

// TestLimiter_AcquireRelease tests basic Acquire and Release operations
func TestLimiter_AcquireRelease(t *testing.T) {
	t.Run("single acquire and release", func(t *testing.T) {
		limiter := NewLimiter(1)

		limiter.Acquire()

		// Channel should be full
		if len(limiter.semaphore) != 1 {
			t.Errorf("semaphore length = %d, want 1", len(limiter.semaphore))
		}

		limiter.Release()

		// Channel should be empty
		if len(limiter.semaphore) != 0 {
			t.Errorf("semaphore length = %d, want 0", len(limiter.semaphore))
		}
	})

	t.Run("multiple acquire and release", func(t *testing.T) {
		limiter := NewLimiter(3)

		limiter.Acquire()
		limiter.Acquire()
		limiter.Acquire()

		// All slots should be taken
		if len(limiter.semaphore) != 3 {
			t.Errorf("semaphore length = %d, want 3", len(limiter.semaphore))
		}

		limiter.Release()
		if len(limiter.semaphore) != 2 {
			t.Errorf("after 1 release: semaphore length = %d, want 2", len(limiter.semaphore))
		}

		limiter.Release()
		if len(limiter.semaphore) != 1 {
			t.Errorf("after 2 releases: semaphore length = %d, want 1", len(limiter.semaphore))
		}

		limiter.Release()
		if len(limiter.semaphore) != 0 {
			t.Errorf("after 3 releases: semaphore length = %d, want 0", len(limiter.semaphore))
		}
	})

	t.Run("acquire blocks when limit reached", func(t *testing.T) {
		limiter := NewLimiter(2)

		// Fill the limiter
		limiter.Acquire()
		limiter.Acquire()

		acquired := make(chan bool, 1)
		go func() {
			limiter.Acquire() // This should block
			acquired <- true
		}()

		// Wait a bit to ensure goroutine is blocked
		select {
		case <-acquired:
			t.Error("Acquire should have blocked but didn't")
		case <-time.After(100 * time.Millisecond):
			// Expected - goroutine is blocked
		}

		// Release one slot
		limiter.Release()

		// Now the blocked goroutine should proceed
		select {
		case <-acquired:
			// Expected - goroutine unblocked
		case <-time.After(100 * time.Millisecond):
			t.Error("Acquire should have unblocked but didn't")
		}

		// Clean up
		limiter.Release()
		limiter.Release()
	})
}

// TestLimiter_ConcurrentOperations tests concurrent usage
func TestLimiter_ConcurrentOperations(t *testing.T) {
	t.Run("respects concurrency limit", func(t *testing.T) {
		const maxConcurrent = 3
		const totalGoroutines = 10

		limiter := NewLimiter(maxConcurrent)
		var (
			currentConcurrent int32
			maxObserved       int32
			wg                sync.WaitGroup
		)

		for i := 0; i < totalGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				limiter.Acquire()
				defer limiter.Release()

				// Increment and check current concurrent count
				current := atomic.AddInt32(&currentConcurrent, 1)

				// Update max observed if necessary
				for {
					old := atomic.LoadInt32(&maxObserved)
					if current <= old || atomic.CompareAndSwapInt32(&maxObserved, old, current) {
						break
					}
				}

				// Simulate work
				time.Sleep(50 * time.Millisecond)

				// Decrement current count
				atomic.AddInt32(&currentConcurrent, -1)
			}()
		}

		wg.Wait()

		max := atomic.LoadInt32(&maxObserved)
		if max > maxConcurrent {
			t.Errorf("max concurrent = %d, want <= %d", max, maxConcurrent)
		}
		if max < maxConcurrent {
			t.Logf("max concurrent observed = %d (expected %d, but acceptable due to timing)", max, maxConcurrent)
		}
	})

	t.Run("handles many goroutines", func(t *testing.T) {
		const maxConcurrent = 5
		const totalGoroutines = 100

		limiter := NewLimiter(maxConcurrent)
		var (
			completed int32
			wg        sync.WaitGroup
		)

		for i := 0; i < totalGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				limiter.Acquire()
				defer limiter.Release()

				// Simulate work
				time.Sleep(10 * time.Millisecond)

				atomic.AddInt32(&completed, 1)
			}()
		}

		wg.Wait()

		if completed != totalGoroutines {
			t.Errorf("completed = %d, want %d", completed, totalGoroutines)
		}
	})

	t.Run("no deadlock with proper release", func(t *testing.T) {
		limiter := NewLimiter(2)
		var wg sync.WaitGroup

		done := make(chan bool)
		go func() {
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					limiter.Acquire()
					defer limiter.Release()
					time.Sleep(10 * time.Millisecond)
				}()
			}
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("deadlock detected - operations did not complete in time")
		}
	})
}

// TestLimiter_EdgeCases tests edge cases
func TestLimiter_EdgeCases(t *testing.T) {
	t.Run("single slot limiter", func(t *testing.T) {
		limiter := NewLimiter(1)
		var (
			order []int
			mu    sync.Mutex
			wg    sync.WaitGroup
		)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			i := i
			go func() {
				defer wg.Done()

				limiter.Acquire()
				defer limiter.Release()

				mu.Lock()
				order = append(order, i)
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)
			}()
		}

		wg.Wait()

		// All should complete
		if len(order) != 5 {
			t.Errorf("completed goroutines = %d, want 5", len(order))
		}
	})

	t.Run("rapid acquire and release", func(t *testing.T) {
		limiter := NewLimiter(10)
		var wg sync.WaitGroup

		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				limiter.Acquire()
				// Release immediately
				limiter.Release()
			}()
		}

		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("rapid operations timed out")
		}
	})

	t.Run("interleaved acquire and release", func(t *testing.T) {
		limiter := NewLimiter(3)

		// Acquire all slots
		limiter.Acquire()
		limiter.Acquire()
		limiter.Acquire()

		// Release and immediately acquire in different order
		limiter.Release()
		limiter.Acquire()

		limiter.Release()
		limiter.Acquire()

		limiter.Release()
		limiter.Release()
		limiter.Release()

		// All slots should be free
		if len(limiter.semaphore) != 0 {
			t.Errorf("semaphore length = %d, want 0", len(limiter.semaphore))
		}
	})
}

// TestLimiter_DeferPattern tests the recommended defer pattern
func TestLimiter_DeferPattern(t *testing.T) {
	t.Run("defer ensures release on panic", func(t *testing.T) {
		limiter := NewLimiter(2)

		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic")
				}
			}()

			limiter.Acquire()
			defer limiter.Release()

			panic("simulated error")
		}()

		// Slot should be released
		if len(limiter.semaphore) != 0 {
			t.Error("slot was not released after panic")
		}
	})

	t.Run("defer ensures release on early return", func(t *testing.T) {
		limiter := NewLimiter(2)

		func() bool {
			limiter.Acquire()
			defer limiter.Release()

			// Early return
			if true {
				return false
			}

			return true
		}()

		// Slot should be released
		if len(limiter.semaphore) != 0 {
			t.Error("slot was not released after early return")
		}
	})
}

// TestLimiter_MultipleRelease tests releasing more than acquired
func TestLimiter_MultipleRelease(t *testing.T) {
	t.Run("release without acquire causes overflow", func(t *testing.T) {
		limiter := NewLimiter(2)

		// This is incorrect usage but should not deadlock
		done := make(chan bool, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// Might panic due to receiving from empty channel
					done <- false
					return
				}
				done <- true
			}()

			// Try to release without acquire
			limiter.Release()
		}()

		select {
		case result := <-done:
			if result {
				t.Log("Release without Acquire completed (will allow extra Acquire)")
			}
		case <-time.After(100 * time.Millisecond):
			// Expected - blocks because channel is empty
			t.Log("Release without Acquire blocked (expected behavior)")
		}
	})
}

// TestLimiter_Fairness tests if limiter is fair (optional, may vary)
func TestLimiter_Fairness(t *testing.T) {
	t.Run("goroutines get fair access", func(t *testing.T) {
		limiter := NewLimiter(1)
		const numGoroutines = 5

		var (
			accessOrder []int
			mu          sync.Mutex
			wg          sync.WaitGroup
		)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			i := i
			go func() {
				defer wg.Done()

				// Small delay to ensure all goroutines start
				time.Sleep(10 * time.Millisecond)

				limiter.Acquire()
				defer limiter.Release()

				mu.Lock()
				accessOrder = append(accessOrder, i)
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)
			}()
		}

		wg.Wait()

		// All goroutines should have accessed
		if len(accessOrder) != numGoroutines {
			t.Errorf("access order length = %d, want %d", len(accessOrder), numGoroutines)
		}

		// Note: Go channels don't guarantee FIFO for multiple waiting goroutines,
		// so we just verify all completed
		seen := make(map[int]bool)
		for _, id := range accessOrder {
			seen[id] = true
		}
		if len(seen) != numGoroutines {
			t.Errorf("unique goroutines = %d, want %d", len(seen), numGoroutines)
		}
	})
}

// BenchmarkLimiter_AcquireRelease benchmarks acquire/release operations
func BenchmarkLimiter_AcquireRelease(b *testing.B) {
	b.Run("sequential", func(b *testing.B) {
		limiter := NewLimiter(1)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			limiter.Acquire()
			limiter.Release()
		}
	})

	b.Run("parallel with limit 10", func(b *testing.B) {
		limiter := NewLimiter(10)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Acquire()
				limiter.Release()
			}
		})
	})

	b.Run("parallel with limit 100", func(b *testing.B) {
		limiter := NewLimiter(100)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Acquire()
				limiter.Release()
			}
		})
	})
}

// BenchmarkLimiter_WithWork benchmarks with simulated work
func BenchmarkLimiter_WithWork(b *testing.B) {
	b.Run("with short work", func(b *testing.B) {
		limiter := NewLimiter(10)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Acquire()
				// Simulate short work
				time.Sleep(1 * time.Microsecond)
				limiter.Release()
			}
		})
	})
}

// ExampleLimiter demonstrates basic usage
func ExampleLimiter() {
	// Create a limiter allowing 3 concurrent operations
	limiter := NewLimiter(3)
	var wg sync.WaitGroup

	// Launch 10 goroutines, but only 3 will run concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			limiter.Acquire()
			defer limiter.Release()

			// Simulate work
			time.Sleep(100 * time.Millisecond)
		}(i)
	}

	wg.Wait()
}

// ExampleLimiter_withContext demonstrates usage with context-like cancellation
func ExampleLimiter_withContext() {
	limiter := NewLimiter(2)
	var wg sync.WaitGroup

	// Simulate cancellable operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			limiter.Acquire()
			defer limiter.Release()

			// Do work
			time.Sleep(50 * time.Millisecond)
		}(i)
	}

	wg.Wait()
}
