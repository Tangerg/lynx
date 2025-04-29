// Package flow provides utilities for managing asynchronous operations with type safety.
//
// This package introduces the AsyncResult type, which represents a value that
// may not yet be available. It provides concurrency-safe mechanisms to set and
// retrieve results, chain operations, and handle errors with proper context cancellation.
//
// AsyncResult implements a promise-like pattern that works with Go's context
// system for cancellation and timeouts. It allows for safe concurrent access
// to results that may be computed by background goroutines.
package flow

import (
	"context"
	"sync"
	"sync/atomic"
)

// AsyncResult represents a value that will be available in the future.
// It provides a type-safe way to work with asynchronous operations.
//
// AsyncResult implements a promise-like pattern with these key features:
// - Generic type parameter for type-safe result handling
// - Context integration for cancellation
// - Thread-safe access to results
// - Ability to chain results to create dependent operations
// - Safe handling of completion state with atomic operations
//
// The zero value is not usable; use NewAsyncResult to create a new instance.
type AsyncResult[T any] struct {
	// ctx is the context that can cancel this async result
	ctx context.Context

	// result stores the value once available
	result T

	// err stores any error that occurred during the operation
	err error

	// mu protects concurrent access to result and err fields
	// using a RWMutex allows multiple readers when retrieving results
	mu sync.RWMutex

	// completionCh is closed when the result is set or the context is canceled
	// this is used for signaling completion to any goroutines waiting for results
	completionCh chan struct{}

	// isCompleted atomically tracks whether the result has been completed
	// using atomic operations allows for fast, lock-free checking of completion status
	isCompleted atomic.Bool

	// completionWg is used to wait for the background completion routine to finish
	// ensures proper synchronization when retrieving results
	completionWg sync.WaitGroup
}

// NewAsyncResult creates a new AsyncResult instance with the given context.
// The context can be used to cancel the operation that will produce the result.
//
// It initializes the necessary synchronization primitives and starts a background
// goroutine to monitor for completion through either context cancellation
// or explicit result setting.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	result := NewAsyncResult[string](ctx)
//
//	// The result will automatically complete with a context.DeadlineExceeded
//	// error if no value is set within 5 seconds
func NewAsyncResult[T any](ctx context.Context) *AsyncResult[T] {
	res := &AsyncResult[T]{
		ctx:          ctx,
		completionCh: make(chan struct{}, 1),
	}
	res.completionWg.Add(1)
	res.awaitCompletion()
	return res
}

// awaitCompletion waits for either the context to be canceled or the result to be set.
// This method runs in a background goroutine started by NewAsyncResult.
//
// When the context is canceled, the AsyncResult is completed with the context's error.
// When the completionCh is closed (by Set/ SetResult/ SetError), the AsyncResult completes
// with the provided value and error.
//
// This method ensures that an AsyncResult always eventually completes, even if
// the producer fails to call Set methods.
func (p *AsyncResult[T]) awaitCompletion() {
	defer func() {
		p.isCompleted.Store(true)
		p.completionWg.Done()
	}()

	select {
	case <-p.ctx.Done():
		p.err = p.ctx.Err()
	case <-p.completionCh:
	}
}

// forkFrom copies the result and error from the parent AsyncResult when it completes.
// This is an internal method used by Chain().
//
// It waits for either:
// - The current context to be canceled, in which case it completes with the context error
// - The parent AsyncResult to complete, in which case it copies the parent's result and error
//
// This method allows for creating dependency chains between AsyncResults without
// blocking the caller.
func (p *AsyncResult[T]) forkFrom(parent *AsyncResult[T]) {
	defer p.markAsCompleted()

	select {
	case <-p.ctx.Done():
		p.err = p.ctx.Err()
	case <-parent.completionCh:
		// Copy the parent's result and error when it completes
		p.result, p.err = parent.result, parent.err
	}
}

// SetResult sets the result value and marks the AsyncResult as completed.
// If the AsyncResult is already completed, this method does nothing.
//
// This is a convenience method that calls Set with a nil error.
//
// Example:
//
//	result := NewAsyncResult[string](ctx)
//	go func() {
//	    // Perform some computation
//	    time.Sleep(time.Second)
//	    result.SetResult("computation complete")
//	}()
//
//	// In another goroutine or later
//	value, err := result.Result() // Will block until result is set or ctx is canceled
func (p *AsyncResult[T]) SetResult(result T) {
	p.Set(result, nil)
}

// SetError sets an error and marks the AsyncResult as completed with that error.
// If the AsyncResult is already completed, this method does nothing.
//
// This is a convenience method that calls Set with a zero value for the result type
// and the provided error.
//
// Example:
//
//	result := NewAsyncResult[string](ctx)
//	go func() {
//	    // Attempt some operation
//	    if err := someRiskyOperation(); err != nil {
//	        result.SetError(err)
//	        return
//	    }
//	    result.SetResult("operation succeeded")
//	}()
func (p *AsyncResult[T]) SetError(err error) {
	var zero T
	p.Set(zero, err)
}

// Set sets both the result value and error, then marks the AsyncResult as completed.
// If the AsyncResult is already completed, this method does nothing.
//
// This is a lower-level method typically used internally or when both a result
// and error need to be set simultaneously. Most callers should use SetResult or
// SetError instead.
//
// The method is thread-safe and can be called from any goroutine.
//
// Example:
//
//	func processData(data []byte) (string, error) {
//	    // Process data and return result or error
//	    if len(data) == 0 {
//	        return "", errors.New("empty data")
//	    }
//	    return string(data), nil
//	}
//
//	result := NewAsyncResult[string](ctx)
//	go func() {
//	    res, err := processData(someData)
//	    result.Set(res, err) // Set both result and error at once
//	}()
func (p *AsyncResult[T]) Set(result T, err error) {
	if p.IsCompleted() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.result = result
	p.err = err
	p.markAsCompleted()
}

// IsCompleted returns true if the AsyncResult has been completed, either with
// a value, an error, or by context cancellation.
//
// This method is thread-safe and non-blocking, making it useful for checking
// completion status without waiting for the result.
//
// Example:
//
//	if result.IsCompleted() {
//	    // We can get the result without blocking
//	    value, err := result.Result()
//	    // Process value or handle error...
//	} else {
//	    // Result is not yet available
//	    // Maybe do something else while waiting...
//	}
func (p *AsyncResult[T]) IsCompleted() bool {
	return p.isCompleted.Load()
}

// Result returns the result value and any error. This method blocks until
// the AsyncResult is completed, either by setting a value/error or by
// context cancellation.
//
// The method is thread-safe and can be called concurrently from multiple goroutines.
// All callers will receive the same result once it's available.
//
// Example:
//
//	result := startAsyncOperation(ctx)
//
//	// This will block until the operation completes
//	value, err := result.Result()
//	if err != nil {
//	    // Handle error, which might be context.Canceled, context.DeadlineExceeded,
//	    // or an error set by SetError
//	    log.Printf("Operation failed: %v", err)
//	    return
//	}
//
//	// Use the value
//	log.Printf("Got result: %v", value)
func (p *AsyncResult[T]) Result() (T, error) {
	p.completionWg.Wait()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.result, p.err
}

// Fork creates a new AsyncResult that will be completed with the same
// result and error as this one. This allows creating dependent operations
// that only proceed when the parent operation completes.
//
// The chained result inherits the same context as the parent, so it will
// be canceled if the parent context is canceled.
//
// This method is useful for creating pipelines of asynchronous operations
// where subsequent steps depend on previous ones.
//
// Example:
//
//	// Start an async operation
//	parent := fetchDataAsync(ctx)
//
//	// Create a dependent operation that waits for the first to complete
//	child := parent.Chain()
//
//	// Start work with the child in another goroutine
//	go func() {
//	    data, err := child.Result() // This blocks until parent completes
//	    if err != nil {
//	        // Handle error
//	        return
//	    }
//	    // Process data further...
//	}()
//
//	// The original goroutine can continue with other work
func (p *AsyncResult[T]) Fork() *AsyncResult[T] {
	res := NewAsyncResult[T](p.ctx)
	go res.forkFrom(p)
	return res
}

// markAsCompleted marks the AsyncResult as completed and signals any waiters.
// This is an internal method used by Set, SetResult, and SetError.
//
// The method ensures that:
// - The completion flag is set atomically
// - The completion channel is closed to signal any waiting goroutines
// - We wait for the background completion routine to finish
//
// This method is idempotent and will do nothing if the AsyncResult is already completed.
func (p *AsyncResult[T]) markAsCompleted() {
	if p.IsCompleted() {
		return
	}
	p.isCompleted.Store(true)
	close(p.completionCh)
	p.completionWg.Wait()
}
