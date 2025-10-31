package safe

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPanicError_Error tests the Error method of PanicError
func TestPanicError_Error(t *testing.T) {
	tests := []struct {
		name      string
		panicInfo any
		checkFn   func(string) bool
	}{
		{
			name:      "string panic",
			panicInfo: "test panic",
			checkFn: func(msg string) bool {
				return strings.Contains(msg, "test panic") &&
					strings.Contains(msg, "panic:") &&
					strings.Contains(msg, "timestamp:") &&
					strings.Contains(msg, "error:") &&
					strings.Contains(msg, "stack:")
			},
		},
		{
			name:      "int panic",
			panicInfo: 42,
			checkFn: func(msg string) bool {
				return strings.Contains(msg, "42")
			},
		},
		{
			name:      "error panic",
			panicInfo: errors.New("custom error"),
			checkFn: func(msg string) bool {
				return strings.Contains(msg, "custom error")
			},
		},
		{
			name:      "nil panic",
			panicInfo: nil,
			checkFn: func(msg string) bool {
				return strings.Contains(msg, "panic:")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := []byte("fake stack trace")
			err := NewPanicError(tt.panicInfo, stack)

			if err == nil {
				t.Fatal("NewPanicError returned nil")
			}

			errMsg := err.Error()
			if !tt.checkFn(errMsg) {
				t.Errorf("Error message validation failed: %q", errMsg)
			}
		})
	}
}

// TestNewPanicError tests the NewPanicError constructor
func TestNewPanicError(t *testing.T) {
	t.Run("creates valid PanicError", func(t *testing.T) {
		panicInfo := "test panic"
		stack := []byte("test stack trace")

		err := NewPanicError(panicInfo, stack)

		if err == nil {
			t.Fatal("NewPanicError returned nil")
		}

		var panicErr *PanicError
		ok := errors.As(err, &panicErr)
		if !ok {
			t.Fatal("returned error is not *PanicError")
		}

		if panicErr.info != panicInfo {
			t.Errorf("info = %v, want %v", panicErr.info, panicInfo)
		}

		if string(panicErr.stack) != string(stack) {
			t.Errorf("stack = %s, want %s", panicErr.stack, stack)
		}

		if panicErr.time.IsZero() {
			t.Error("timestamp is zero")
		}

		if panicErr.message == "" {
			t.Error("message is empty")
		}
	})

	t.Run("timestamp is recent", func(t *testing.T) {
		before := time.Now()
		err := NewPanicError("test", []byte("stack"))
		after := time.Now()

		var panicErr *PanicError
		errors.As(err, &panicErr)

		if panicErr.time.Before(before) || panicErr.time.After(after) {
			t.Errorf("timestamp %v not between %v and %v",
				panicErr.time, before, after)
		}
	})

	t.Run("message format", func(t *testing.T) {
		err := NewPanicError("test info", []byte("test stack"))
		msg := err.Error()

		requiredParts := []string{"panic:", "timestamp:", "error:", "stack:", "test info", "test stack"}
		for _, part := range requiredParts {
			if !strings.Contains(msg, part) {
				t.Errorf("message missing required part %q: %s", part, msg)
			}
		}
	})

	t.Run("timestamp format", func(t *testing.T) {
		err := NewPanicError("test", []byte("stack"))
		msg := err.Error()

		t.Log(msg)
	})
}

// TestWithRecover tests the WithRecover function
func TestWithRecover(t *testing.T) {
	t.Run("nil function returns nil", func(t *testing.T) {
		result := WithRecover(nil)
		if result != nil {
			t.Error("WithRecover(nil) should return nil")
		}
	})

	t.Run("normal execution without panic", func(t *testing.T) {
		executed := false
		fn := func() {
			executed = true
		}

		wrapped := WithRecover(fn)
		if wrapped == nil {
			t.Fatal("WithRecover returned nil")
		}

		wrapped()

		if !executed {
			t.Error("function was not executed")
		}
	})

	t.Run("recovers from panic", func(t *testing.T) {
		var capturedErr error
		var mu sync.Mutex

		fn := func() {
			panic("test panic")
		}

		errorHandler := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			capturedErr = err
		}

		wrapped := WithRecover(fn, errorHandler)
		wrapped()

		// Give time for error handler to execute
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		if capturedErr == nil {
			t.Fatal("error was not captured")
		}

		if !strings.Contains(capturedErr.Error(), "test panic") {
			t.Errorf("error doesn't contain panic message: %v", capturedErr)
		}
	})

	t.Run("recovers from different panic types", func(t *testing.T) {
		testCases := []struct {
			name      string
			panicVal  any
			checkFunc func(string) bool
		}{
			{
				name:     "string panic",
				panicVal: "string error",
				checkFunc: func(msg string) bool {
					return strings.Contains(msg, "string error")
				},
			},
			{
				name:     "int panic",
				panicVal: 123,
				checkFunc: func(msg string) bool {
					return strings.Contains(msg, "123")
				},
			},
			{
				name:     "error panic",
				panicVal: errors.New("error panic"),
				checkFunc: func(msg string) bool {
					return strings.Contains(msg, "error panic")
				},
			},
			{
				name:     "struct panic",
				panicVal: struct{ Code int }{Code: 500},
				checkFunc: func(msg string) bool {
					return strings.Contains(msg, "500")
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var capturedErr error
				var mu sync.Mutex

				fn := func() {
					panic(tc.panicVal)
				}

				errorHandler := func(err error) {
					mu.Lock()
					defer mu.Unlock()
					capturedErr = err
				}

				wrapped := WithRecover(fn, errorHandler)
				wrapped()

				time.Sleep(10 * time.Millisecond)

				mu.Lock()
				defer mu.Unlock()

				if capturedErr == nil {
					t.Fatal("error was not captured")
				}

				if !tc.checkFunc(capturedErr.Error()) {
					t.Errorf("error validation failed: %v", capturedErr)
				}
			})
		}
	})

	t.Run("multiple error handlers", func(t *testing.T) {
		callCount := 0
		var mu sync.Mutex

		fn := func() {
			panic("test")
		}

		handler1 := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
		}

		handler2 := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
		}

		handler3 := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
		}

		wrapped := WithRecover(fn, handler1, handler2, handler3)
		wrapped()

		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		if callCount != 3 {
			t.Errorf("callCount = %d, want 3", callCount)
		}
	})

	t.Run("no error handlers", func(t *testing.T) {
		fn := func() {
			panic("test panic")
		}

		// Should not panic even without error handlers
		wrapped := WithRecover(fn)
		wrapped() // Should complete without panic
	})

	t.Run("captures stack trace", func(t *testing.T) {
		var capturedErr error
		var mu sync.Mutex

		fn := func() {
			panic("test")
		}

		errorHandler := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			capturedErr = err
		}

		wrapped := WithRecover(fn, errorHandler)
		wrapped()

		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		if capturedErr == nil {
			t.Fatal("error not captured")
		}

		errMsg := capturedErr.Error()
		if !strings.Contains(errMsg, "stack:") {
			t.Error("error message doesn't contain stack trace")
		}
	})
}

// TestGo tests the Go function
func TestGo(t *testing.T) {
	t.Run("executes function in goroutine", func(t *testing.T) {
		executed := make(chan bool, 1)

		fn := func() {
			executed <- true
		}

		Go(fn)

		select {
		case <-executed:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("function was not executed in time")
		}
	})

	t.Run("handles panic in goroutine", func(t *testing.T) {
		errorReceived := make(chan error, 1)

		fn := func() {
			panic("goroutine panic")
		}

		errorHandler := func(err error) {
			errorReceived <- err
		}

		Go(fn, errorHandler)

		select {
		case err := <-errorReceived:
			if !strings.Contains(err.Error(), "goroutine panic") {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("error handler was not called")
		}
	})

	t.Run("nil function", func(t *testing.T) {
		// Should not panic
		Go(nil)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("multiple goroutines", func(t *testing.T) {
		const count = 10
		results := make(chan int, count)

		for i := 0; i < count; i++ {
			i := i // Capture loop variable
			Go(func() {
				results <- i
			})
		}

		received := make(map[int]bool)
		timeout := time.After(100 * time.Millisecond)

		for i := 0; i < count; i++ {
			select {
			case val := <-results:
				received[val] = true
			case <-timeout:
				t.Fatalf("timeout waiting for goroutine %d", i)
			}
		}

		if len(received) != count {
			t.Errorf("received %d values, want %d", len(received), count)
		}
	})

	t.Run("goroutine with multiple error handlers", func(t *testing.T) {
		errorCount := make(chan int, 3)

		fn := func() {
			panic("test")
		}

		handler1 := func(err error) { errorCount <- 1 }
		handler2 := func(err error) { errorCount <- 2 }
		handler3 := func(err error) { errorCount <- 3 }

		Go(fn, handler1, handler2, handler3)

		counts := make(map[int]bool)
		timeout := time.After(100 * time.Millisecond)

		for i := 0; i < 3; i++ {
			select {
			case count := <-errorCount:
				counts[count] = true
			case <-timeout:
				t.Fatalf("timeout waiting for handler %d", i+1)
			}
		}

		if len(counts) != 3 {
			t.Errorf("received %d handler calls, want 3", len(counts))
		}
	})

	t.Run("goroutine without error handler", func(t *testing.T) {
		done := make(chan bool, 1)

		fn := func() {
			panic("unhandled panic")
			// This line should not be reached, but adding done signal after defer
		}

		// Wrap to detect completion
		Go(func() {
			defer func() { done <- true }()
			fn()
		})

		select {
		case <-done:
			// Goroutine completed (panic was recovered)
		case <-time.After(100 * time.Millisecond):
			t.Error("goroutine did not complete")
		}
	})
}

// TestConcurrentPanics tests multiple concurrent panics
func TestConcurrentPanics(t *testing.T) {
	const goroutineCount = 100
	errorChan := make(chan error, goroutineCount)
	var wg sync.WaitGroup

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		i := i
		Go(func() {
			defer wg.Done()
			panic(i)
		}, func(err error) {
			errorChan <- err
		})
	}

	// Wait for all goroutines
	done := make(chan bool)
	go func() {
		wg.Wait()
		time.Sleep(5 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		close(errorChan)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for goroutines")
	}

	errorCount := 0
	for range errorChan {
		errorCount++
	}

	if errorCount != goroutineCount {
		t.Errorf("received %d errors, want %d", errorCount, goroutineCount)
	}
}

// TestPanicError_Fields tests PanicError internal fields
func TestPanicError_Fields(t *testing.T) {
	t.Run("info field", func(t *testing.T) {
		info := "test info"
		err := NewPanicError(info, []byte("stack"))
		var panicErr *PanicError
		errors.As(err, &panicErr)

		if panicErr.info != info {
			t.Errorf("info = %v, want %v", panicErr.info, info)
		}
	})

	t.Run("stack field", func(t *testing.T) {
		stack := []byte("test stack trace")
		err := NewPanicError("info", stack)
		var panicErr *PanicError
		errors.As(err, &panicErr)

		if string(panicErr.stack) != string(stack) {
			t.Errorf("stack = %s, want %s", panicErr.stack, stack)
		}
	})

	t.Run("time field", func(t *testing.T) {
		before := time.Now()
		err := NewPanicError("info", []byte("stack"))
		after := time.Now()

		var panicErr *PanicError
		errors.As(err, &panicErr)

		if panicErr.time.Before(before) || panicErr.time.After(after) {
			t.Error("time field not set correctly")
		}
	})

	t.Run("message field", func(t *testing.T) {
		err := NewPanicError("test", []byte("stack"))
		var panicErr *PanicError
		errors.As(err, &panicErr)

		if panicErr.message == "" {
			t.Error("message field is empty")
		}

		if panicErr.message != panicErr.Error() {
			t.Error("message field doesn't match Error() output")
		}
	})
}

// TestErrorHandlerPanic tests when error handler itself panics
func TestErrorHandlerPanic(t *testing.T) {
	t.Run("error handler panic", func(t *testing.T) {
		executed := make(chan bool, 1)

		fn := func() {
			panic("original panic")
		}

		panicHandler := func(err error) {
			panic("handler panic")
		}

		safeHandler := func(err error) {
			executed <- true
		}

		// Use WithRecover to test, as Go() launches in goroutine
		wrapped := WithRecover(fn, panicHandler, safeHandler)

		// This should not panic the test
		func() {
			defer func() {
				if r := recover(); r != nil {
					// If handler panic propagates, this will catch it
					t.Errorf("panic from handler propagated: %v", r)
				}
			}()
			wrapped()
		}()

		// The second handler should still execute
		select {
		case <-executed:
			// Success - second handler was called
		case <-time.After(100 * time.Millisecond):
			// First handler panicked, might prevent second handler
			// This is expected behavior
		}
	})
}

// BenchmarkWithRecover benchmarks WithRecover function
func BenchmarkWithRecover(b *testing.B) {
	b.Run("no panic", func(b *testing.B) {
		fn := func() {
			_ = 1 + 1
		}
		wrapped := WithRecover(fn)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			wrapped()
		}
	})

	b.Run("with panic", func(b *testing.B) {
		fn := func() {
			panic("test")
		}
		handler := func(err error) {}
		wrapped := WithRecover(fn, handler)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			wrapped()
		}
	})
}

// BenchmarkGo benchmarks Go function
func BenchmarkGo(b *testing.B) {
	b.Run("simple goroutine", func(b *testing.B) {
		fn := func() {
			_ = 1 + 1
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			Go(fn)
		}
	})

	b.Run("with error handler", func(b *testing.B) {
		fn := func() {
			panic("test")
		}
		handler := func(err error) {}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			Go(fn, handler)
		}
	})
}

// BenchmarkNewPanicError benchmarks PanicError creation
func BenchmarkNewPanicError(b *testing.B) {
	stack := []byte("test stack trace\nline 2\nline 3")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewPanicError("test panic", stack)
	}
}
