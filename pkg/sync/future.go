package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrFutureCancelled is returned when a future is cancelled before completion.
// This error indicates that the task was explicitly cancelled by calling Cancel().
var ErrFutureCancelled = errors.New("future cancelled")

// ErrFutureTimedOut is returned when a future operation couldn't complete within
// the specified timeout duration. This typically occurs when GetWithTimeout() is called
// and the time limit expires.
var ErrFutureTimedOut = errors.New("future timed out")

// FutureState represents the current execution state of a Future.
// A Future transitions through various states during its lifecycle:
// New -> Running -> (Success|Failed|Cancelled)
type FutureState int32

// int32 returns the state as an int32 value for internal atomic operations.
func (s FutureState) int32() int32 {
	return int32(s)
}

// IsCreated returns true if the future is in the new state.
// A future is in the new state when it has been created but execution has not started.
func (s FutureState) IsCreated() bool {
	return s == FutureStateCreated
}

// IsRunning returns true if the future is in the running state.
// A future is in the running state when its task is currently executing.
func (s FutureState) IsRunning() bool {
	return s == FutureStateRunning
}

// IsSucceeded returns true if the future has completed successfully.
// A future is in the success state when its task has completed without errors.
func (s FutureState) IsSucceeded() bool {
	return s == FutureStateSucceeded
}

// IsFailed returns true if the future has failed.
// A future is in the failed state when its task completed with an error.
func (s FutureState) IsFailed() bool {
	return s == FutureStateFailed
}

// IsCancelled returns true if the future has been cancelled.
// A future is in the cancelled state when Cancel() was called before completion.
func (s FutureState) IsCancelled() bool {
	return s == FutureStateCancelled
}

// Predefined FutureState constants representing different states of a Future.
const (
	FutureStateCreated   FutureState = iota // Future has been created but not started
	FutureStateRunning                      // Future is currently running
	FutureStateSucceeded                    // Future has completed successfully
	FutureStateFailed                       // Future has completed with an error
	FutureStateCancelled                    // Future has been cancelled
)

// Future represents an asynchronous computation with a type-safe result.
// The generic type parameter V represents the type of the result that will
// be produced when the computation completes.
//
// The Future interface provides methods to check the status of the computation,
// wait for its completion, and retrieve its result.
type Future[V any] interface {
	// Cancel attempts to cancel the execution of this future.
	// Returns true if the future was successfully cancelled by this call.
	//
	// If mayInterruptIfRunning is false, the future will only be cancelled if
	// execution hasn't begun. If true, the currently executing task will be
	// interrupted if possible.
	//
	// Once a future is cancelled, it transitions to the cancelled state and
	// any call to Get() will return ErrFutureCancelled.
	//
	// Returns false if the task has already completed (succeeded, failed, or
	// was already cancelled).
	Cancel(mayInterruptIfRunning bool) bool

	// IsCancelled returns true if this future was cancelled before it completed.
	IsCancelled() bool

	// IsDone returns true if this future completed, either successfully, with an
	// error, or by cancellation.
	//
	// IsDone will be true once the task has finished executing, regardless of the
	// outcome.
	IsDone() bool

	// Get waits if necessary for the computation to complete, and then retrieves its result.
	// Returns the computed result and any error that occurred during computation.
	//
	// This method will block indefinitely until the task completes. Use GetWithTimeout
	// or GetWithContext for versions with timeout capabilities.
	Get() (V, error)

	// GetWithTimeout waits if necessary for at most the given timeout for the computation to complete,
	// and then retrieves its result, if available.
	//
	// If the task completes before the timeout expires, this method returns the result.
	// If the timeout expires, the task is cancelled and ErrFutureTimedOut is returned.
	GetWithTimeout(timeout time.Duration) (V, error)

	// GetWithContext waits if necessary until the context is done for the computation to complete,
	// and then retrieves its result, if available.
	//
	// If the task completes before the context is done, this method returns the result.
	// If the context is cancelled or times out, the task is cancelled and the context error
	// is joined with any existing error.
	GetWithContext(ctx context.Context) (V, error)

	// TryGet attempts to retrieve the result without blocking.
	// Returns the result, error, and a boolean indicating whether the future is done.
	// If the future is not done, the boolean will be false and result/error should be ignored.
	TryGet() (V, error, bool)

	// State returns the current state of the future.
	// This can be used to check the current execution state without blocking.
	State() FutureState
}

// FutureTask implements the Future interface and represents a cancellable asynchronous computation.
// It provides a concrete implementation for executing tasks, checking their state, and retrieving results.
//
// FutureTask is thread-safe and can be safely used from multiple goroutines.
type FutureTask[V any] struct {
	task      func(interrupt <-chan struct{}) (V, error) // The task to execute
	state     atomic.Int32                               // Current state of the future
	value     V                                          // Output value (valid only if state is Success)
	error     error                                      // Error result (valid only if state is Failed or Cancelled)
	done      chan struct{}                              // Channel closed when the task completes
	interrupt chan struct{}                              // Channel closed to interrupt the task
	runOnce   sync.Once                                  // Ensures the task is executed only once
	doneOnce  sync.Once                                  // Ensures completion is processed only once
}

// NewFutureTask creates a new FutureTask that will execute the given task when Run is called.
// The task function receives an interrupt channel that will be closed if the future is cancelled.
// This allows tasks to be made cancellable by periodically checking the interrupt channel.
//
// Panics if task is nil.
//
// Example:
//
//	task := func(interrupt <-chan struct{}) (string, error) {
//	    // Simulate work
//	    select {
//	    case <-time.After(5 * time.Second):
//	        return "Task completed", nil
//	    case <-interrupt:
//	        return "", errors.New("task was interrupted")
//	    }
//	}
//
//	future := NewFutureTask(task)
//	// Note: the task doesn't start until Run() is called
func NewFutureTask[V any](task func(interrupt <-chan struct{}) (V, error)) *FutureTask[V] {
	if task == nil {
		panic("task is nil")
	}
	return &FutureTask[V]{
		task:      task,
		done:      make(chan struct{}),
		interrupt: make(chan struct{}),
	}
}

// NewFutureTaskAndRun creates a new FutureTask and immediately submits it to the default pool for execution.
// The task function receives an interrupt channel that will be closed if the future is cancelled.
//
// This is a convenience method that combines creating a future and scheduling it for execution.
//
// Example:
//
//	future, _ := NewFutureTaskAndRun(func(interrupt <-chan struct{}) (int, error) {
//	    // Do calculation
//	    for i := 0; i < 10; i++ {
//	        select {
//	        case <-interrupt:
//	            return 0, errors.New("calculation interrupted")
//	        default:
//	            // Do work step
//	            time.Sleep(100 * time.Millisecond)
//	        }
//	    }
//	    return 42, nil
//	})
//
//	// Later, get the result
//	result, err := future.Get()
func NewFutureTaskAndRun[V any](task func(interrupt <-chan struct{}) (V, error)) (*FutureTask[V], error) {
	return NewFutureTaskAndRunWithPool(task, DefaultPool())
}

// NewFutureTaskAndRunWithPool creates a new FutureTask and immediately submits it to the specified pool for execution.
// The task function receives an interrupt channel that will be closed if the future is cancelled.
//
// This allows using a custom thread pool for task execution, which can be useful for controlling
// concurrency and resource utilization.
//
// Panics if pool is nil.
//
// Example:
//
//	// Create a custom pool with 5 workers
//	pool := NewFixedPool(5)
//	defer pool.Shutdown()
//
//	future, _ := NewFutureTaskAndRunWithPool(
//	    func(interrupt <-chan struct{}) (float64, error) {
//	        // Perform complex calculation
//	        return 3.14159, nil
//	    },
//	    pool,
//	)
//
//	// Get result with a timeout
//	result, err := future.GetWithTimeout(2 * time.Second)
func NewFutureTaskAndRunWithPool[V any](task func(interrupt <-chan struct{}) (V, error), pool Pool) (*FutureTask[V], error) {
	if pool == nil {
		panic("pool is nil")
	}
	f := NewFutureTask(task)
	err := pool.Submit(f.Run)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// complete sets the final result of the future task, either success or failure.
// This is called internally when the task completes its execution.
//
// This method ensures that the task completion is processed only once, even if
// called from multiple goroutines, and properly updates the future's state.
func (f *FutureTask[V]) complete(v V, err error) {
	f.doneOnce.Do(func() {
		if err != nil {
			f.state.Store(FutureStateFailed.int32())
			f.error = err
		} else {
			f.state.Store(FutureStateSucceeded.int32())
			f.value = v
		}
		close(f.done)

		select {
		case <-f.interrupt:
		default:
			close(f.interrupt)
		}
	})
}

// Run executes the task if it hasn't already started.
// This method is safe to call multiple times - the task will only execute once.
//
// Run transitions the future from New to Running state, executes the task,
// and then transitions to either Success or Failed state depending on the outcome.
//
// Example:
//
//	future := NewFutureTask(func(interrupt <-chan struct{}) (string, error) {
//	    return "Hello, World!", nil
//	})
//
//	// Start the task execution
//	go future.Run()
//
//	// Later
//	result, err := future.Get()
func (f *FutureTask[V]) Run() {
	if !f.State().IsCreated() {
		return
	}
	f.runOnce.Do(func() {
		if f.state.CompareAndSwap(FutureStateCreated.int32(), FutureStateRunning.int32()) {
			v, err := f.task(f.interrupt)
			f.complete(v, err)
		}
	})
}

// Cancel attempts to cancel execution of this task.
// Returns true if this call successfully cancelled the task.
// Returns false if the task has already completed (succeeded, failed, or was already cancelled).
//
// If mayInterruptIfRunning is true, the thread executing this task will be interrupted
// by closing the interrupt channel, which the task should periodically check.
// If false, the task will only be cancelled if it hasn't started running yet.
//
// Example:
//
//	future := NewFutureTaskAndRun(func(interrupt <-chan struct{}) (string, error) {
//	    for i := 0; i < 10; i++ {
//	        select {
//	        case <-interrupt:
//	            return "", errors.New("task cancelled")
//	        default:
//	            time.Sleep(100 * time.Millisecond)
//	        }
//	    }
//	    return "Completed", nil
//	})
//
//	// Try to cancel the task, interrupting it if running
//	cancelled := future.Cancel(true)
//	fmt.Println("Cancelled:", cancelled)
func (f *FutureTask[V]) Cancel(mayInterruptIfRunning bool) bool {
	cancelled := false
	f.doneOnce.Do(func() {
		f.state.Store(FutureStateCancelled.int32())
		f.error = ErrFutureCancelled
		if mayInterruptIfRunning {
			close(f.interrupt) // notify task to interrupt
		}
		close(f.done)
		cancelled = true
	})
	return cancelled
}

// IsCancelled returns true if this task was cancelled before it completed normally.
//
// Example:
//
//	if future.IsCancelled() {
//	    fmt.Println("The task was cancelled")
//	}
func (f *FutureTask[V]) IsCancelled() bool {
	return f.State().IsCancelled()
}

// IsDone returns true if this task completed, either normally or by an exception,
// or was cancelled.
//
// This is useful for checking if a task has completed without blocking, unlike Get()
// which blocks until completion.
//
// Example:
//
//	// Non-blocking check if the task is done
//	if future.IsDone() {
//	    result, err := future.Get() // Won't block now
//	    fmt.Println("Output:", result, "Error:", err)
//	} else {
//	    fmt.Println("Task is still running")
//	}
func (f *FutureTask[V]) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// Get waits if necessary for the computation to complete, and then retrieves its result.
// Returns the computed result and any error that occurred during computation.
//
// This method blocks indefinitely until the task completes. If you need to limit the wait time,
// use GetWithTimeout or GetWithContext instead.
//
// Example:
//
//	future := NewFutureTaskAndRun(func(interrupt <-chan struct{}) (int, error) {
//	    time.Sleep(2 * time.Second)
//	    return 42, nil
//	})
//
//	// This will block until the task completes
//	result, err := future.Get()
//	if err != nil {
//	    log.Fatalf("Task failed: %v", err)
//	}
//	fmt.Println("Output:", result) // Output: Output: 42
func (f *FutureTask[V]) Get() (V, error) {
	<-f.done
	return f.value, f.error
}

// GetWithTimeout waits if necessary for at most the given timeout for the computation to complete,
// and then retrieves its result, if available.
//
// If the timeout expires before the task completes, the task is cancelled and ErrFutureTimedOut
// is returned. A zero or negative timeout will immediately check the current state without waiting.
//
// Example:
//
//	future := NewFutureTaskAndRun(func(interrupt <-chan struct{}) (string, error) {
//	    time.Sleep(5 * time.Second)
//	    return "Completed", nil
//	})
//
//	// Only wait for up to 2 seconds
//	result, err := future.GetWithTimeout(2 * time.Second)
//	if err == ErrFutureTimedOut {
//	    fmt.Println("Task timed out")
//	} else if err != nil {
//	    fmt.Println("Task failed:", err)
//	} else {
//	    fmt.Println("Got result:", result)
//	}
func (f *FutureTask[V]) GetWithTimeout(timeout time.Duration) (V, error) {
	if timeout <= 0 {
		return f.tryGetOrTimeout()
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-f.done:
		return f.value, f.error
	case <-timer.C:
		select {
		case <-f.done:
			return f.value, f.error
		default:
			f.Cancel(true)
			var zero V
			return zero, ErrFutureTimedOut
		}
	}
}

// tryGetOrTimeout is a helper method that attempts to get the result immediately
// or returns a timeout error if not done.
func (f *FutureTask[V]) tryGetOrTimeout() (V, error) {
	select {
	case <-f.done:
		return f.value, f.error
	default:
		f.Cancel(true)
		var zero V
		return zero, ErrFutureTimedOut
	}
}

// GetWithContext waits if necessary until the context is done for the computation to complete,
// and then retrieves its result, if available.
//
// If the task completes before the context is done, this method returns the result.
// If the context is cancelled or times out, the task is cancelled and the context error
// is joined with any existing error.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
//	defer cancel()
//
//	future := NewFutureTaskAndRun(func(interrupt <-chan struct{}) (float64, error) {
//	    // Simulate a long calculation
//	    time.Sleep(5 * time.Second)
//	    return 3.14159, nil
//	})
//
//	// Wait for completion or context cancellation/timeout
//	result, err := future.GetWithContext(ctx)
//	if err != nil {
//	    if errors.Is(err, context.DeadlineExceeded) {
//	        fmt.Println("Operation timed out")
//	    } else {
//	        fmt.Println("Error:", err)
//	    }
//	} else {
//	    fmt.Println("Output:", result)
//	}
func (f *FutureTask[V]) GetWithContext(ctx context.Context) (V, error) {
	select {
	case <-f.done:
		return f.value, f.error
	case <-ctx.Done():
		select {
		case <-f.done:
			return f.value, f.error
		default:
			f.Cancel(true)
			var zero V
			return zero, errors.Join(f.error, ctx.Err())
		}
	}
}

// TryGet attempts to retrieve the result without blocking.
// Returns the result, error, and a boolean indicating whether the future is done.
//
// If the boolean is false, the future is not done and the result/error should be ignored.
// If the boolean is true, the future has completed and result/error are valid.
//
// This is useful when you want to check if a result is available without waiting.
//
// Example:
//
//	result, err, ok := future.TryGet()
//	if ok {
//	    if err != nil {
//	        fmt.Println("Task failed:", err)
//	    } else {
//	        fmt.Println("Got result:", result)
//	    }
//	} else {
//	    fmt.Println("Task is still running")
//	}
func (f *FutureTask[V]) TryGet() (V, error, bool) {
	select {
	case <-f.done:
		return f.value, f.error, true
	default:
		var zero V
		return zero, nil, false
	}
}

// State returns the current state of the future.
// This can be used to check the execution status without blocking.
//
// Example:
//
//	state := future.State()
//	switch {
//	case state.IsCreated():
//	    fmt.Println("Task hasn't started yet")
//	case state.IsRunning():
//	    fmt.Println("Task is currently running")
//	case state.IsSucceeded():
//	    fmt.Println("Task completed successfully")
//	case state.IsFailed():
//	    fmt.Println("Task failed")
//	case state.IsCancelled():
//	    fmt.Println("Task was cancelled")
//	}
func (f *FutureTask[V]) State() FutureState {
	return FutureState(f.state.Load())
}
