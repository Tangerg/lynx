package sync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestFutureState tests the FutureState type and its methods
func TestFutureState(t *testing.T) {
	t.Run("int32 conversion", func(t *testing.T) {
		states := []FutureState{
			FutureStateCreated,
			FutureStateRunning,
			FutureStateSucceeded,
			FutureStateFailed,
			FutureStateCancelled,
		}
		for i, state := range states {
			if state.int32() != int32(i) {
				t.Errorf("Expected int32() to return %d, got %d", i, state.int32())
			}
		}
	})

	t.Run("IsCreated", func(t *testing.T) {
		if !FutureStateCreated.IsCreated() {
			t.Error("FutureStateCreated.IsCreated() should return true")
		}
		if FutureStateRunning.IsCreated() {
			t.Error("FutureStateRunning.IsCreated() should return false")
		}
	})

	t.Run("IsRunning", func(t *testing.T) {
		if !FutureStateRunning.IsRunning() {
			t.Error("FutureStateRunning.IsRunning() should return true")
		}
		if FutureStateCreated.IsRunning() {
			t.Error("FutureStateCreated.IsRunning() should return false")
		}
	})

	t.Run("IsSucceeded", func(t *testing.T) {
		if !FutureStateSucceeded.IsSucceeded() {
			t.Error("FutureStateSucceeded.IsSucceeded() should return true")
		}
		if FutureStateFailed.IsSucceeded() {
			t.Error("FutureStateFailed.IsSucceeded() should return false")
		}
	})

	t.Run("IsFailed", func(t *testing.T) {
		if !FutureStateFailed.IsFailed() {
			t.Error("FutureStateFailed.IsFailed() should return true")
		}
		if FutureStateSucceeded.IsFailed() {
			t.Error("FutureStateSucceeded.IsFailed() should return false")
		}
	})

	t.Run("IsCancelled", func(t *testing.T) {
		if !FutureStateCancelled.IsCancelled() {
			t.Error("FutureStateCancelled.IsCancelled() should return true")
		}
		if FutureStateRunning.IsCancelled() {
			t.Error("FutureStateRunning.IsCancelled() should return false")
		}
	})
}

// TestNewFutureTask tests the creation of FutureTask
func TestNewFutureTask(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		if future == nil {
			t.Fatal("NewFutureTask returned nil")
		}
		if !future.State().IsCreated() {
			t.Error("New future should be in Created state")
		}
	})

	t.Run("panic on nil task", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when task is nil")
			}
		}()
		NewFutureTask[int](nil)
	})
}

// TestFutureTaskRun tests the Run method
func TestFutureTaskRun(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		executed := false
		task := func(interrupt <-chan struct{}) (string, error) {
			executed = true
			return "success", nil
		}
		future := NewFutureTask(task)
		future.Run()

		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != "success" {
			t.Errorf("Expected 'success', got '%s'", result)
		}
		if !executed {
			t.Error("Task was not executed")
		}
		if !future.State().IsSucceeded() {
			t.Error("Future should be in Succeeded state")
		}
	})

	t.Run("execution with error", func(t *testing.T) {
		expectedErr := errors.New("task error")
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, expectedErr
		}
		future := NewFutureTask(task)
		future.Run()

		result, err := future.Get()
		if err != expectedErr {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
		if result != 0 {
			t.Errorf("Expected zero value, got %d", result)
		}
		if !future.State().IsFailed() {
			t.Error("Future should be in Failed state")
		}
	})

	t.Run("run only once", func(t *testing.T) {
		counter := atomic.Int32{}
		task := func(interrupt <-chan struct{}) (int, error) {
			counter.Add(1)
			return 1, nil
		}
		future := NewFutureTask(task)

		// Run multiple times concurrently
		done := make(chan struct{})
		for i := 0; i < 10; i++ {
			go func() {
				future.Run()
				done <- struct{}{}
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		if counter.Load() != 1 {
			t.Errorf("Task executed %d times, expected 1", counter.Load())
		}
	})

	t.Run("run after cancelled", func(t *testing.T) {
		executed := false
		task := func(interrupt <-chan struct{}) (int, error) {
			executed = true
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Cancel(false)
		future.Run()

		if executed {
			t.Error("Task should not execute after cancellation")
		}
	})
}

// TestFutureTaskCancel tests the Cancel method
func TestFutureTaskCancel(t *testing.T) {
	t.Run("cancel before run", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		cancelled := future.Cancel(false)

		if !cancelled {
			t.Error("Cancel should return true")
		}
		if !future.IsCancelled() {
			t.Error("Future should be cancelled")
		}

		result, err := future.Get()
		if err != ErrFutureCancelled {
			t.Errorf("Expected ErrFutureCancelled, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected zero value, got %d", result)
		}
	})

	t.Run("cancel during execution", func(t *testing.T) {
		started := make(chan struct{})
		task := func(interrupt <-chan struct{}) (int, error) {
			close(started)
			select {
			case <-interrupt:
				return 0, errors.New("interrupted")
			case <-time.After(5 * time.Second):
				return 42, nil
			}
		}
		future := NewFutureTask(task)
		go future.Run()

		<-started // Wait for task to start
		time.Sleep(10 * time.Millisecond)
		cancelled := future.Cancel(true)

		if !cancelled {
			t.Error("Cancel should return true during execution")
		}

		result, err := future.Get()
		if result != 0 || err == nil {
			t.Errorf("Expected error, got result=%d, err=%v", result, err)
		}
	})

	t.Run("cancel after completion", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Run()
		future.Get() // Wait for completion

		cancelled := future.Cancel(true)
		if cancelled {
			t.Error("Cancel should return false after completion")
		}
		if future.IsCancelled() {
			t.Error("Future should not be cancelled after completion")
		}
	})

	t.Run("multiple cancel calls", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)

		cancelled1 := future.Cancel(false)
		cancelled2 := future.Cancel(false)
		cancelled3 := future.Cancel(true)

		if !cancelled1 {
			t.Error("First cancel should return true")
		}
		if cancelled2 {
			t.Error("Second cancel should return false")
		}
		if cancelled3 {
			t.Error("Third cancel should return false")
		}
	})
}

// TestFutureTaskGet tests the Get method
func TestFutureTaskGet(t *testing.T) {
	t.Run("get successful result", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (string, error) {
			return "hello", nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != "hello" {
			t.Errorf("Expected 'hello', got '%s'", result)
		}
	})

	t.Run("get error result", func(t *testing.T) {
		expectedErr := errors.New("test error")
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, expectedErr
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.Get()
		if err != expectedErr {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("get cancelled result", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			<-time.After(10 * time.Second)
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Cancel(false)

		result, err := future.Get()
		if err != ErrFutureCancelled {
			t.Errorf("Expected ErrFutureCancelled, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("multiple get calls", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 100, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result1, err1 := future.Get()
		result2, err2 := future.Get()
		result3, err3 := future.Get()

		if result1 != 100 || result2 != 100 || result3 != 100 {
			t.Error("All Get calls should return same result")
		}
		if err1 != nil || err2 != nil || err3 != nil {
			t.Error("All Get calls should return no error")
		}
	})
}

// TestFutureTaskGetWithTimeout tests the GetWithTimeout method
func TestFutureTaskGetWithTimeout(t *testing.T) {
	t.Run("success before timeout", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(50 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithTimeout(200 * time.Millisecond)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("timeout expires", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			select {
			case <-interrupt:
				return 0, errors.New("interrupted")
			case <-time.After(5 * time.Second):
				return 42, nil
			}
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithTimeout(50 * time.Millisecond)
		if err != ErrFutureTimedOut {
			t.Errorf("Expected ErrFutureTimedOut, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
		if !future.IsCancelled() {
			t.Error("Future should be cancelled after timeout")
		}
	})

	t.Run("zero timeout", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithTimeout(0)
		if err != ErrFutureTimedOut {
			t.Errorf("Expected ErrFutureTimedOut, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithTimeout(-1 * time.Second)
		if err != ErrFutureTimedOut {
			t.Errorf("Expected ErrFutureTimedOut, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("race condition near timeout", func(t *testing.T) {
		// This tests the double-check logic in GetWithTimeout
		successes := 0
		timeouts := 0
		iterations := 100

		for i := 0; i < iterations; i++ {
			task := func(interrupt <-chan struct{}) (int, error) {
				time.Sleep(45 * time.Millisecond)
				return 42, nil
			}
			future := NewFutureTask(task)
			go future.Run()

			_, err := future.GetWithTimeout(50 * time.Millisecond)
			if err == nil {
				successes++
			} else if err == ErrFutureTimedOut {
				timeouts++
			}
		}

		// Both outcomes are possible due to timing, just ensure no crashes
		if successes+timeouts != iterations {
			t.Errorf("Expected %d results, got %d", iterations, successes+timeouts)
		}
	})
}

// TestFutureTaskGetWithContext tests the GetWithContext method
func TestFutureTaskGetWithContext(t *testing.T) {
	t.Run("success before context cancel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(50 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithContext(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		task := func(interrupt <-chan struct{}) (int, error) {
			select {
			case <-interrupt:
				return 0, errors.New("interrupted")
			case <-time.After(5 * time.Second):
				return 42, nil
			}
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithContext(ctx)
		if err == nil {
			t.Error("Expected error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		task := func(interrupt <-chan struct{}) (int, error) {
			select {
			case <-interrupt:
				return 0, errors.New("interrupted")
			case <-time.After(5 * time.Second):
				return 42, nil
			}
		}
		future := NewFutureTask(task)
		go future.Run()

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := future.GetWithContext(ctx)
		if err == nil {
			t.Error("Expected error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("already cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err := future.GetWithContext(ctx)
		if err == nil {
			t.Error("Expected error")
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})
}

// TestFutureTaskTryGet tests the TryGet method
func TestFutureTaskTryGet(t *testing.T) {
	t.Run("not done yet", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		result, err, ok := future.TryGet()
		if ok {
			t.Error("TryGet should return false when not done")
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("done successfully", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Run()

		// Wait a bit to ensure completion
		time.Sleep(10 * time.Millisecond)

		result, err, ok := future.TryGet()
		if !ok {
			t.Error("TryGet should return true when done")
		}
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("done with error", func(t *testing.T) {
		expectedErr := errors.New("task error")
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, expectedErr
		}
		future := NewFutureTask(task)
		future.Run()

		time.Sleep(10 * time.Millisecond)

		result, err, ok := future.TryGet()
		if !ok {
			t.Error("TryGet should return true when done")
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
		if err != expectedErr {
			t.Errorf("Expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Cancel(false)

		result, err, ok := future.TryGet()
		if !ok {
			t.Error("TryGet should return true when cancelled")
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
		if err != ErrFutureCancelled {
			t.Errorf("Expected ErrFutureCancelled, got %v", err)
		}
	})
}

// TestFutureTaskIsDone tests the IsDone method
func TestFutureTaskIsDone(t *testing.T) {
	t.Run("not done", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		if future.IsDone() {
			t.Error("IsDone should return false when not complete")
		}
	})

	t.Run("done successfully", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Run()

		time.Sleep(10 * time.Millisecond)

		if !future.IsDone() {
			t.Error("IsDone should return true when complete")
		}
	})

	t.Run("done with error", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, errors.New("error")
		}
		future := NewFutureTask(task)
		future.Run()

		time.Sleep(10 * time.Millisecond)

		if !future.IsDone() {
			t.Error("IsDone should return true when complete with error")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Cancel(false)

		if !future.IsDone() {
			t.Error("IsDone should return true when cancelled")
		}
	})
}

// TestFutureTaskState tests the State method
func TestFutureTaskState(t *testing.T) {
	t.Run("created state", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)

		if !future.State().IsCreated() {
			t.Error("New future should be in Created state")
		}
	})

	t.Run("running state", func(t *testing.T) {
		started := make(chan struct{})
		proceed := make(chan struct{})
		task := func(interrupt <-chan struct{}) (int, error) {
			close(started)
			<-proceed
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		<-started
		if !future.State().IsRunning() {
			t.Error("Future should be in Running state during execution")
		}
		close(proceed)
	})

	t.Run("succeeded state", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Run()
		future.Get()

		if !future.State().IsSucceeded() {
			t.Error("Future should be in Succeeded state after successful completion")
		}
	})

	t.Run("failed state", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, errors.New("error")
		}
		future := NewFutureTask(task)
		future.Run()
		future.Get()

		if !future.State().IsFailed() {
			t.Error("Future should be in Failed state after error")
		}
	})

	t.Run("cancelled state", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Cancel(false)

		if !future.State().IsCancelled() {
			t.Error("Future should be in Cancelled state after cancellation")
		}
	})
}

// TestFutureTaskConcurrency tests concurrent operations
func TestFutureTaskConcurrency(t *testing.T) {
	t.Run("concurrent gets", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(50 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		done := make(chan struct{})
		results := make([]int, 10)
		errors := make([]error, 10)

		for i := 0; i < 10; i++ {
			go func(index int) {
				results[index], errors[index] = future.Get()
				done <- struct{}{}
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		for i := 0; i < 10; i++ {
			if results[i] != 42 {
				t.Errorf("Goroutine %d: expected 42, got %d", i, results[i])
			}
			if errors[i] != nil {
				t.Errorf("Goroutine %d: expected nil error, got %v", i, errors[i])
			}
		}
	})

	t.Run("concurrent cancel", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(100 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)

		cancelCount := atomic.Int32{}
		done := make(chan struct{})

		for i := 0; i < 10; i++ {
			go func() {
				if future.Cancel(true) {
					cancelCount.Add(1)
				}
				done <- struct{}{}
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		if cancelCount.Load() != 1 {
			t.Errorf("Expected exactly 1 successful cancel, got %d", cancelCount.Load())
		}
	})

	t.Run("concurrent state checks", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			time.Sleep(50 * time.Millisecond)
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		done := make(chan struct{})
		for i := 0; i < 100; i++ {
			go func() {
				_ = future.State()
				_ = future.IsDone()
				_ = future.IsCancelled()
				done <- struct{}{}
			}()
		}

		for i := 0; i < 100; i++ {
			<-done
		}

		// Should not crash
	})
}

// TestFutureTaskInterrupt tests interrupt handling
func TestFutureTaskInterrupt(t *testing.T) {
	t.Run("task respects interrupt", func(t *testing.T) {
		interrupted := atomic.Bool{}
		task := func(interrupt <-chan struct{}) (int, error) {
			for i := 0; i < 100; i++ {
				select {
				case <-interrupt:
					interrupted.Store(true)
					return 0, errors.New("interrupted")
				default:
					time.Sleep(10 * time.Millisecond)
				}
			}
			return 42, nil
		}
		future := NewFutureTask(task)
		go future.Run()

		time.Sleep(10 * time.Millisecond)
		future.Cancel(true)
		time.Sleep(20 * time.Millisecond)
		future.Get()

		if !interrupted.Load() {
			t.Error("Task should have been interrupted")
		}
	})
}

// TestFutureTaskEdgeCases tests edge cases
func TestFutureTaskEdgeCases(t *testing.T) {
	t.Run("task panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate")
			}
		}()

		task := func(interrupt <-chan struct{}) (int, error) {
			panic("task panic")
		}
		future := NewFutureTask(task)
		future.Run()
	})

	t.Run("zero value result", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 0, nil
		}
		future := NewFutureTask(task)
		future.Run()

		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("nil error with zero value", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (*string, error) {
			return nil, nil
		}
		future := NewFutureTask(task)
		future.Run()

		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("very quick task", func(t *testing.T) {
		task := func(interrupt <-chan struct{}) (int, error) {
			return 42, nil
		}
		future := NewFutureTask(task)
		future.Run()

		// Immediately try to get
		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})
}

// TestNewFutureTaskAndRun tests the convenience constructor
func TestNewFutureTaskAndRun(t *testing.T) {
	t.Run("creates and runs task", func(t *testing.T) {
		executed := atomic.Bool{}
		task := func(interrupt <-chan struct{}) (string, error) {
			executed.Store(true)
			return "success", nil
		}

		future, err := NewFutureTaskAndRun(task)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != "success" {
			t.Errorf("Expected 'success', got '%s'", result)
		}
		if !executed.Load() {
			t.Error("Task should have been executed")
		}
	})
}

// Benchmark tests
func BenchmarkFutureTaskCreate(b *testing.B) {
	task := func(interrupt <-chan struct{}) (int, error) {
		return 42, nil
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewFutureTask(task)
	}
}

func BenchmarkFutureTaskRunAndGet(b *testing.B) {
	task := func(interrupt <-chan struct{}) (int, error) {
		return 42, nil
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		future := NewFutureTask(task)
		future.Run()
		future.Get()
	}
}

func BenchmarkFutureTaskConcurrentGet(b *testing.B) {
	task := func(interrupt <-chan struct{}) (int, error) {
		return 42, nil
	}
	future := NewFutureTask(task)
	future.Run()
	future.Get() // Complete it first

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			future.Get()
		}
	})
}

func BenchmarkFutureTaskState(b *testing.B) {
	task := func(interrupt <-chan struct{}) (int, error) {
		return 42, nil
	}
	future := NewFutureTask(task)
	future.Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = future.State()
	}
}

func BenchmarkFutureTaskTryGet(b *testing.B) {
	task := func(interrupt <-chan struct{}) (int, error) {
		return 42, nil
	}
	future := NewFutureTask(task)
	future.Run()
	future.Get()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		future.TryGet()
	}
}
