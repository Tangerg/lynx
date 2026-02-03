package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ========================================
// Mock Future Implementation for Testing
// ========================================

// mockFuture is a simple Future implementation for testing purposes
type mockFuture[V any] struct {
	mu     sync.Mutex
	done   chan struct{}
	value  V
	err    error
	ready  bool
	closed bool
}

// newMockFuture creates a new mock future
func newMockFuture[V any]() *mockFuture[V] {
	return &mockFuture[V]{
		done: make(chan struct{}),
	}
}

// Complete sets the result and marks the future as ready
func (f *mockFuture[V]) Complete(value V, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return
	}

	f.value = value
	f.err = err
	f.ready = true
	f.closed = true
	close(f.done)
}

// Get blocks until the result is available
func (f *mockFuture[V]) Get() (V, error) {
	<-f.done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value, f.err
}

// GetWithTimeout blocks until the result is available or timeout expires
func (f *mockFuture[V]) GetWithTimeout(timeout time.Duration) (V, error) {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.value, f.err
	case <-time.After(timeout):
		var zero V
		return zero, errors.New("timeout exceeded")
	}
}

// GetWithContext blocks until the result is available or context is cancelled
func (f *mockFuture[V]) GetWithContext(ctx context.Context) (V, error) {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.value, f.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// TryGet attempts to retrieve the result without blocking
func (f *mockFuture[V]) TryGet() (value V, err error, ready bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value, f.err, f.ready
}

// ========================================
// Helper function to create async processor
// ========================================

func createAsyncProcessor[I, O any](fn func(I) (O, error),
	delay time.Duration,
) func(context.Context, I) (*mockFuture[O], error) {
	return func(ctx context.Context, input I) (*mockFuture[O], error) {
		future := newMockFuture[O]()

		go func() {
			if delay > 0 {
				time.Sleep(delay)
			}
			result, err := fn(input)
			future.Complete(result, err)
		}()

		return future, nil
	}
}

// ========================================
// Async Node Tests
// ========================================

// TestNewAsync tests the constructor for Async
func TestNewAsync(t *testing.T) {
	tests := []struct {
		name      string
		processor func(context.Context, int) (*mockFuture[int], error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid processor",
			processor: func(ctx context.Context, i int) (*mockFuture[int], error) {
				return newMockFuture[int](), nil
			},
			wantErr: false,
		},
		{
			name:      "nil processor",
			processor: nil,
			wantErr:   true,
			errMsg:    "async processor cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			async, err := NewAsync(tt.processor)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAsync() expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewAsync() error = %v, want %v", err.Error(), tt.errMsg)
				}
				if async != nil {
					t.Errorf("NewAsync() expected nil async on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewAsync() unexpected error = %v", err)
				}
				if async == nil {
					t.Errorf("NewAsync() returned nil async")
				}
			}
		})
	}
}

// TestAsync_Run tests basic async execution
func TestAsync_Run(t *testing.T) {
	tests := []struct {
		name      string
		processor func(context.Context, int) (*mockFuture[int], error)
		input     int
		wantValue int
		wantErr   bool
		errSubstr string
	}{
		{
			name: "successful immediate completion",
			processor: createAsyncProcessor(func(i int) (int, error) {
				return i * 2, nil
			}, 0),
			input:     5,
			wantValue: 10,
			wantErr:   false,
		},
		{
			name: "successful delayed completion",
			processor: createAsyncProcessor(func(i int) (int, error) {
				return i * 3, nil
			}, 10*time.Millisecond),
			input:     5,
			wantValue: 15,
			wantErr:   false,
		},
		{
			name: "async operation with error",
			processor: createAsyncProcessor(func(i int) (int, error) {
				return 0, errors.New("computation failed")
			}, 0),
			input:   5,
			wantErr: true,
		},
		{
			name: "processor returns error",
			processor: func(ctx context.Context, i int) (*mockFuture[int], error) {
				return nil, errors.New("failed to start")
			},
			input:     5,
			wantErr:   true,
			errSubstr: "failed to start async operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			async, err := NewAsync(tt.processor)
			if err != nil {
				t.Fatalf("NewAsync() error = %v", err)
			}

			ctx := context.Background()
			future, err := async.Run(ctx, tt.input)

			// Check if Run itself returns an error
			if tt.errSubstr != "" {
				if err == nil {
					t.Errorf("Run() expected error containing %q", tt.errSubstr)
				} else if err.Error() != tt.errSubstr && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Run() error = %v, want substring %v", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Run() unexpected error = %v", err)
			}

			// Get the result from the future
			result, err := future.Get()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Future.Get() expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Future.Get() unexpected error = %v", err)
				}
				if result != tt.wantValue {
					t.Errorf("Future.Get() = %v, want %v", result, tt.wantValue)
				}
			}
		})
	}
}

// TestFuture_Get tests blocking Get method
func TestFuture_Get(t *testing.T) {
	t.Run("get completed future", func(t *testing.T) {
		future := newMockFuture[int]()
		future.Complete(42, nil)

		result, err := future.Get()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if result != 42 {
			t.Errorf("Get() = %v, want 42", result)
		}
	})

	t.Run("get future with error", func(t *testing.T) {
		future := newMockFuture[int]()
		expectedErr := errors.New("computation error")
		future.Complete(0, expectedErr)

		_, err := future.Get()
		if err == nil {
			t.Errorf("Get() expected error but got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("Get() error = %v, want %v", err, expectedErr)
		}
	})

	t.Run("get blocks until completion", func(t *testing.T) {
		future := newMockFuture[int]()

		done := make(chan bool)
		go func() {
			time.Sleep(20 * time.Millisecond)
			future.Complete(100, nil)
		}()

		go func() {
			result, err := future.Get()
			if err != nil {
				t.Errorf("Get() error = %v", err)
			}
			if result != 100 {
				t.Errorf("Get() = %v, want 100", result)
			}
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Get() did not complete in time")
		}
	})
}

// TestFuture_GetWithTimeout tests timeout behavior
func TestFuture_GetWithTimeout(t *testing.T) {
	t.Run("get before timeout", func(t *testing.T) {
		future := newMockFuture[int]()

		go func() {
			time.Sleep(10 * time.Millisecond)
			future.Complete(42, nil)
		}()

		result, err := future.GetWithTimeout(50 * time.Millisecond)
		if err != nil {
			t.Errorf("GetWithTimeout() error = %v", err)
		}
		if result != 42 {
			t.Errorf("GetWithTimeout() = %v, want 42", result)
		}
	})

	t.Run("timeout exceeded", func(t *testing.T) {
		future := newMockFuture[int]()

		go func() {
			time.Sleep(100 * time.Millisecond)
			future.Complete(42, nil)
		}()

		_, err := future.GetWithTimeout(20 * time.Millisecond)
		if err == nil {
			t.Errorf("GetWithTimeout() expected timeout error")
		}
	})

	t.Run("immediate completion", func(t *testing.T) {
		future := newMockFuture[int]()
		future.Complete(42, nil)

		result, err := future.GetWithTimeout(10 * time.Millisecond)
		if err != nil {
			t.Errorf("GetWithTimeout() error = %v", err)
		}
		if result != 42 {
			t.Errorf("GetWithTimeout() = %v, want 42", result)
		}
	})
}

// TestFuture_GetWithContext tests context-based cancellation
func TestFuture_GetWithContext(t *testing.T) {
	t.Run("get before context cancellation", func(t *testing.T) {
		future := newMockFuture[int]()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(10 * time.Millisecond)
			future.Complete(42, nil)
		}()

		result, err := future.GetWithContext(ctx)
		if err != nil {
			t.Errorf("GetWithContext() error = %v", err)
		}
		if result != 42 {
			t.Errorf("GetWithContext() = %v, want 42", result)
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		future := newMockFuture[int]()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(100 * time.Millisecond)
			future.Complete(42, nil)
		}()

		_, err := future.GetWithContext(ctx)
		if err == nil {
			t.Errorf("GetWithContext() expected context error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("GetWithContext() error = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("context with value", func(t *testing.T) {
		type contextKey string
		key := contextKey("test")

		future := newMockFuture[string]()
		ctx := context.WithValue(context.Background(), key, "test_value")

		future.Complete("result", nil)

		result, err := future.GetWithContext(ctx)
		if err != nil {
			t.Errorf("GetWithContext() error = %v", err)
		}
		if result != "result" {
			t.Errorf("GetWithContext() = %v, want 'result'", result)
		}

		// Verify context value is accessible
		if ctx.Value(key).(string) != "test_value" {
			t.Errorf("context value lost")
		}
	})
}

// TestFuture_TryGet tests non-blocking retrieval
func TestFuture_TryGet(t *testing.T) {
	t.Run("try get not ready", func(t *testing.T) {
		future := newMockFuture[int]()

		_, _, ready := future.TryGet()
		if ready {
			t.Errorf("TryGet() ready = true, want false")
		}
	})

	t.Run("try get ready with value", func(t *testing.T) {
		future := newMockFuture[int]()
		future.Complete(42, nil)

		value, err, ready := future.TryGet()
		if !ready {
			t.Errorf("TryGet() ready = false, want true")
		}
		if err != nil {
			t.Errorf("TryGet() error = %v", err)
		}
		if value != 42 {
			t.Errorf("TryGet() value = %v, want 42", value)
		}
	})

	t.Run("try get ready with error", func(t *testing.T) {
		future := newMockFuture[int]()
		expectedErr := errors.New("test error")
		future.Complete(0, expectedErr)

		_, err, ready := future.TryGet()
		if !ready {
			t.Errorf("TryGet() ready = false, want true")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("TryGet() error = %v, want %v", err, expectedErr)
		}
	})

	t.Run("polling pattern", func(t *testing.T) {
		future := newMockFuture[int]()

		go func() {
			time.Sleep(30 * time.Millisecond)
			future.Complete(100, nil)
		}()

		// Poll until ready
		var value int
		var err error
		var ready bool
		attempts := 0

		for attempts < 10 {
			value, err, ready = future.TryGet()
			if ready {
				break
			}
			attempts++
			time.Sleep(10 * time.Millisecond)
		}

		if !ready {
			t.Errorf("TryGet() never became ready after %d attempts", attempts)
		}
		if err != nil {
			t.Errorf("TryGet() error = %v", err)
		}
		if value != 100 {
			t.Errorf("TryGet() value = %v, want 100", value)
		}
	})
}

// TestAsync_ComplexScenarios tests real-world usage patterns
func TestAsync_ComplexScenarios(t *testing.T) {
	t.Run("multiple async operations", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 10*time.Millisecond)

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		ctx := context.Background()

		// Start multiple async operations
		future1, err := async.Run(ctx, 1)
		if err != nil {
			t.Fatalf("Run(1) error = %v", err)
		}

		future2, err := async.Run(ctx, 2)
		if err != nil {
			t.Fatalf("Run(2) error = %v", err)
		}

		future3, err := async.Run(ctx, 3)
		if err != nil {
			t.Fatalf("Run(3) error = %v", err)
		}

		// Collect results
		result1, _ := future1.Get()
		result2, _ := future2.Get()
		result3, _ := future3.Get()

		if result1 != 2 {
			t.Errorf("result1 = %v, want 2", result1)
		}
		if result2 != 4 {
			t.Errorf("result2 = %v, want 4", result2)
		}
		if result3 != 6 {
			t.Errorf("result3 = %v, want 6", result3)
		}
	})

	t.Run("async with type conversion", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (string, error) {
			return string(rune('A' + i)), nil
		}, 5*time.Millisecond)

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		ctx := context.Background()
		future, err := async.Run(ctx, 0)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if result != "A" {
			t.Errorf("result = %v, want 'A'", result)
		}
	})

	t.Run("async with struct types", func(t *testing.T) {
		type Request struct {
			ID   int
			Data string
		}
		type Response struct {
			ID     int
			Result string
		}

		processor := createAsyncProcessor(func(req Request) (Response, error) {
			return Response{
				ID:     req.ID,
				Result: "processed_" + req.Data,
			}, nil
		}, 10*time.Millisecond)

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		ctx := context.Background()
		req := Request{ID: 123, Data: "test"}
		future, err := async.Run(ctx, req)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		resp, err := future.Get()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if resp.ID != 123 {
			t.Errorf("response.ID = %v, want 123", resp.ID)
		}
		if resp.Result != "processed_test" {
			t.Errorf("response.Result = %v, want 'processed_test'", resp.Result)
		}
	})

	t.Run("async operation cancellation", func(t *testing.T) {
		processor := func(ctx context.Context, i int) (*mockFuture[int], error) {
			future := newMockFuture[int]()

			go func() {
				select {
				case <-time.After(100 * time.Millisecond):
					future.Complete(i*2, nil)
				case <-ctx.Done():
					future.Complete(0, ctx.Err())
				}
			}()

			return future, nil
		}

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		future, err := async.Run(ctx, 5)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		_, err = future.GetWithContext(ctx)
		if err == nil {
			t.Errorf("GetWithContext() expected context error")
		}
	})

	t.Run("chained async operations", func(t *testing.T) {
		// First async: double the value
		processor1 := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 10*time.Millisecond)

		async1, _ := NewAsync(processor1)

		// Second async: add 10 to the value
		processor2 := createAsyncProcessor(func(i int) (int, error) {
			return i + 10, nil
		}, 10*time.Millisecond)

		async2, _ := NewAsync(processor2)

		ctx := context.Background()

		// Chain: input -> async1 -> async2
		future1, _ := async1.Run(ctx, 5)
		intermediate, _ := future1.Get()

		future2, _ := async2.Run(ctx, intermediate)
		final, _ := future2.Get()

		// (5 * 2) + 10 = 20
		if final != 20 {
			t.Errorf("final result = %v, want 20", final)
		}
	})
}

// TestAsync_EdgeCases tests edge cases and boundary conditions
func TestAsync_EdgeCases(t *testing.T) {
	t.Run("zero value input", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 0)

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		ctx := context.Background()
		future, err := async.Run(ctx, 0)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if result != 0 {
			t.Errorf("result = %v, want 0", result)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 0)

		async, err := NewAsync(processor)
		if err != nil {
			t.Fatalf("NewAsync() error = %v", err)
		}

		// This should work with nil context (though not recommended)
		future, err := async.Run(nil, 5)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if result != 10 {
			t.Errorf("result = %v, want 10", result)
		}
	})

	t.Run("multiple get calls", func(t *testing.T) {
		future := newMockFuture[int]()
		future.Complete(42, nil)

		// Multiple Get calls should return the same result
		result1, _ := future.Get()
		result2, _ := future.Get()
		result3, _ := future.Get()

		if result1 != 42 || result2 != 42 || result3 != 42 {
			t.Errorf("multiple Get() calls returned inconsistent results")
		}
	})

	t.Run("concurrent get calls", func(t *testing.T) {
		future := newMockFuture[int]()

		done := make(chan int, 3)

		// Start multiple goroutines waiting for the result
		for i := 0; i < 3; i++ {
			go func() {
				result, _ := future.Get()
				done <- result
			}()
		}

		time.Sleep(10 * time.Millisecond)
		future.Complete(42, nil)

		// All goroutines should receive the result
		for i := 0; i < 3; i++ {
			select {
			case result := <-done:
				if result != 42 {
					t.Errorf("concurrent Get() = %v, want 42", result)
				}
			case <-time.After(100 * time.Millisecond):
				t.Errorf("concurrent Get() timed out")
			}
		}
	})
}

// TestAsync_PerformanceCharacteristics tests performance aspects
func TestAsync_PerformanceCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	t.Run("async returns immediately", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 100*time.Millisecond) // Long delay

		async, _ := NewAsync(processor)

		start := time.Now()
		_, err := async.Run(context.Background(), 5)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		// Run should return almost immediately, not wait for completion
		if duration > 20*time.Millisecond {
			t.Errorf("Run() took %v, should return immediately", duration)
		}
	})

	t.Run("concurrent async operations", func(t *testing.T) {
		processor := createAsyncProcessor(func(i int) (int, error) {
			return i * 2, nil
		}, 50*time.Millisecond)

		async, _ := NewAsync(processor)
		ctx := context.Background()

		// Start 10 async operations
		futures := make([]*mockFuture[int], 10)
		start := time.Now()

		for i := 0; i < 10; i++ {
			future, _ := async.Run(ctx, i)
			futures[i] = future
		}

		// Wait for all to complete
		for i, future := range futures {
			result, _ := future.Get()
			if result != i*2 {
				t.Errorf("future[%d] = %v, want %v", i, result, i*2)
			}
		}

		duration := time.Since(start)

		// Should complete in ~50ms (concurrent), not 500ms (sequential)
		if duration > 100*time.Millisecond {
			t.Logf("concurrent execution took %v (expected ~50ms)", duration)
		}
	})
}
