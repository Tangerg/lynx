// Package future provides a generic implementation of the Future pattern for asynchronous computation.
// It allows running tasks concurrently and retrieving their results later, with support for
// timeouts, cancellation, and context integration.
//
// This implementation is inspired by the java.util.concurrent.Future interface but adapted for
// Go's concurrency model and idiomatic error handling.
package future

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrCancelled is returned when a Future is explicitly canceled.
	// This is analogous to Java's CancellationException.
	ErrCancelled = errors.New("future cancelled")

	// ErrTimedOut is returned when a Future operation times out.
	// This is analogous to Java's TimeoutException.
	ErrTimedOut = errors.New("future timeout")
)

// State represents the current state of a Future.
// This is analogous to the Future.State enum in Java 19+.
type State int32

// int32 converts the State enum to its int32 representation for atomic operations.
func (s State) int32() int32 {
	return int32(s)
}

// Future state constants representing the different lifecycle phases.
// These match Java's Future.State enum values, with an additional 'New' state.
const (
	New       State = iota // The Future has been created but not started (Go-specific)
	Running                // The Future's task is currently executing (equivalent to Java's RUNNING)
	Success                // The Future's task completed successfully (equivalent to Java's SUCCESS)
	Failed                 // The Future's task completed with an error (equivalent to Java's FAILED)
	Cancelled              // The Future was canceled before completion (equivalent to Java's CANCELLED)
)

// Future represents an asynchronous computation that may produce a value in the future.
// It is a generic type that can hold any value type, similar to Java's Future<V>.
//
// Unlike Java's Future which uses exceptions for error handling, this implementation
// follows Go's convention of returning errors alongside values.
type Future[V any] struct {
	task      Task[V]       // The function to execute asynchronously
	state     atomic.Int32  // The current state of the Future, using atomic operations
	value     V             // The result value (valid only after completion)
	error     error         // Any error that occurred during execution
	done      chan struct{} // Channel that is closed when the Future completes
	interrupt chan struct{} // Channel that is closed to signal cancellation
	once      sync.Once     // Ensures one-time execution of completion logic
}

// Task is a function that represents the computation to be performed.
// It receives an interrupt channel that signals when the task should be canceled.
// This is analogous to Java's Callable<V> but with built-in cancellation support.
type Task[V any] func(interrupt <-chan struct{}) (V, error)

// NewFuture creates a new Future with the given task but does not start execution.
// It returns both the Future and a function that will start the task when called.
//
// This is similar to creating a FutureTask in Java without submitting it to an Executor.
func NewFuture[V any](task Task[V]) (*Future[V], func()) {
	if task == nil {
		panic("task is nil")
	}
	f := &Future[V]{
		done:      make(chan struct{}, 1),
		interrupt: make(chan struct{}, 1),
		task:      task,
	}
	return f, f.run
}

// NewFutureAndRun creates a new Future and immediately schedules its execution
// using the default goroutine pool.
//
// This is a convenience method equivalent to calling NewFutureAndRunWithPool
// with the package's default pool.
func NewFutureAndRun[V any](task Task[V]) *Future[V] {
	return NewFutureAndRunWithPool(task, DefaultPool())
}

// NewFutureAndRunWithPool creates a new Future and immediately schedules its execution
// using the provided goroutine pool.
//
// This is analogous to Java's ExecutorService.submit(Callable<V>) which returns a Future<V>.
func NewFutureAndRunWithPool[V any](task Task[V], pool Pool) *Future[V] {
	if pool == nil {
		panic("pool must not be nil")
	}
	f, run := NewFuture(task)
	pool.Go(run)
	return f
}

// run executes the Future's task if it's in the New state.
// This transitions the Future to the Running state.
func (f *Future[V]) run() {
	if f.state.CompareAndSwap(New.int32(), Running.int32()) {
		f.complete(f.task(f.interrupt))
	}
}

// complete stores the result of the computation and updates the Future's state.
// It ensures these operations happen exactly once.
func (f *Future[V]) complete(value V, err error) {
	f.once.Do(func() {
		f.value = value
		f.error = err
		if err != nil {
			f.state.CompareAndSwap(Running.int32(), Failed.int32())
		} else {
			f.state.CompareAndSwap(Running.int32(), Success.int32())
		}
		close(f.done)
	})
}

// cancel attempts to cancel the Future if it's currently running.
// It may optionally interrupt the running task.
func (f *Future[V]) cancel(mayInterruptIfRunning bool) {
	if !f.state.CompareAndSwap(Running.int32(), Cancelled.int32()) {
		return
	}
	f.error = ErrCancelled
	if mayInterruptIfRunning {
		close(f.interrupt)
	}
	close(f.done)
}

// Cancel attempts to cancel execution of the Future.
// If mayInterruptIfRunning is true, it will also signal the running task to stop.
// Returns true if the Future was successfully cancelled or was already cancelled.
//
// This is equivalent to Java's Future.cancel(boolean mayInterruptIfRunning) method.
func (f *Future[V]) Cancel(mayInterruptIfRunning bool) bool {
	f.once.Do(func() {
		f.cancel(mayInterruptIfRunning)
	})
	return f.IsCancelled()
}

// IsCancelled returns true if the Future is in the Cancelled state.
//
// This is equivalent to Java's Future.isCancelled() method.
func (f *Future[V]) IsCancelled() bool {
	return f.state.Load() == Cancelled.int32()
}

// IsDone returns true if the Future has completed (successfully, with an error, or was cancelled).
//
// This is equivalent to Java's Future.isDone() method.
func (f *Future[V]) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// Get blocks until the Future completes and returns the result.
// If the Future completed with an error, that error is returned.
//
// This is equivalent to Java's Future.get() method, but returns errors
// directly instead of throwing exceptions.
func (f *Future[V]) Get() (V, error) {
	<-f.done
	return f.value, f.error
}

// GetWithTimeout waits for the Future to complete but only up to the specified timeout.
// If the timeout expires, the Future is cancelled and an ErrTimeout is returned.
//
// This is equivalent to Java's Future.get(long timeout, TimeUnit unit) method,
// but uses Go's time.Duration instead of separate timeout and unit parameters.
func (f *Future[V]) GetWithTimeout(timeout time.Duration) (V, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-f.done:
		return f.value, f.error
	case <-timer.C:
		f.Cancel(true)
		return f.value, errors.Join(f.error, ErrTimedOut)
	}
}

// GetWithContext waits for the Future to complete but respects context cancellation.
// If the context is cancelled, the Future is also cancelled and the context error is returned.
//
// This is a Go-specific extension that has no direct equivalent in Java's Future interface,
// but provides integration with Go's context mechanism.
func (f *Future[V]) GetWithContext(ctx context.Context) (V, error) {
	select {
	case <-f.done:
		return f.value, f.error
	case <-ctx.Done():
		f.Cancel(true)
		return f.value, errors.Join(f.error, ctx.Err())
	}
}

// State returns the current state of the Future.
//
// This is equivalent to Java 19+'s Future.state() method.
func (f *Future[V]) State() State {
	return State(f.state.Load())
}

// ResultNow returns the computed result immediately without waiting.
// This method is intended for situations where the caller knows that
// the task has already completed successfully.
//
// It panics in the following cases:
// - If the task has not yet completed
// - If the task completed with an error
// - If the task was cancelled
//
// Unlike Get(), this method does not block, as it's meant to be used
// only when task completion is already confirmed.
//
// This is equivalent to Java 19+'s Future.resultNow() method, but uses
// panic instead of exceptions for error conditions.
func (f *Future[V]) ResultNow() V {
	if !f.IsDone() {
		panic("Task has not completed")
	}
	value, err := f.Get()
	if err != nil {
		panic("Task did not complete with a result")
	}
	return value
}

// ErrorNow returns the error thrown by the task immediately without waiting.
// This method is intended for situations where the caller knows that
// the task has already completed with an error.
//
// It panics in the following cases:
// - If the task has not yet completed
// - If the task completed successfully (with a result)
// - If the task was cancelled
//
// Unlike Get(), this method does not block, as it's meant to be used
// only when task failure is already confirmed.
//
// This is analogous to Java 19+'s Future.exceptionNow() method, but returns
// Go errors instead of Java exceptions and uses panic for error conditions.
func (f *Future[V]) ErrorNow() error {
	if !f.IsDone() {
		panic("Task has not completed")
	}
	if f.IsCancelled() {
		panic("Task was cancelled")
	}
	_, err := f.Get()
	if err == nil {
		panic("Task completed with a result")
	}
	return err
}
