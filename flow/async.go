package flow

import (
	"context"
	"sync"
	"sync/atomic"
)

// ReadonlyAsyncResult represents a value that will be available in the future.
// It provides a type-safe way to work with asynchronous operations.
//
// ReadonlyAsyncResult implements a promise-like pattern with these key features:
// - Generic type parameter for type-safe result handling
// - Context integration for cancellation
// - Thread-safe access to results
// - Ability to fork results to create dependent operations
// - Safe handling of completion state with atomic operations
//
// The zero value is not usable; use NewAsyncResult to create a new instance.
type ReadonlyAsyncResult[T any] struct {
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

// NewReadonlyAsyncResult creates a new ReadonlyAsyncResult instance with the given context.
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
func NewReadonlyAsyncResult[T any](ctx context.Context) *ReadonlyAsyncResult[T] {
	res := &ReadonlyAsyncResult[T]{
		ctx:          ctx,
		completionCh: make(chan struct{}),
	}
	res.completionWg.Add(1)
	go res.awaitCompletion()
	return res
}

// awaitCompletion waits for either the context to be canceled or the result to be set.
// This method runs in a background goroutine started by NewAsyncResult.
//
// When the context is canceled, the ReadonlyAsyncResult is completed with the context's error.
// When the completionCh is closed (by Set/SetResult/SetError), the ReadonlyAsyncResult completes
// with the provided value and error.
//
// This method ensures that an ReadonlyAsyncResult always eventually completes, even if
// the producer fails to call Set methods.
func (a *ReadonlyAsyncResult[T]) awaitCompletion() {
	defer func() {
		a.isCompleted.Store(true)
		a.completionWg.Done()
	}()

	select {
	case <-a.ctx.Done():
		a.mu.Lock()
		a.err = a.ctx.Err()
		a.mu.Unlock()
	case <-a.completionCh:
	}
}

// forkFrom copies the result and error from the parent ReadonlyAsyncResult when it completes.
// This is an internal method used by Fork().
//
// It waits for either:
// - The current context to be canceled, in which case it completes with the context error
// - The parent ReadonlyAsyncResult to complete, in which case it copies the parent's result and error
//
// This method allows for creating dependency chains between AsyncResults without
// blocking the caller.
func (a *ReadonlyAsyncResult[T]) forkFrom(parent *ReadonlyAsyncResult[T]) {
	defer a.markAsCompleted()

	select {
	case <-a.ctx.Done():
		a.mu.Lock()
		a.err = a.ctx.Err()
		a.mu.Unlock()
	case <-parent.completionCh:
		// Copy the parent's result and error when it completes
		parent.mu.RLock()
		a.mu.Lock()
		a.result, a.err = parent.result, parent.err
		a.mu.Unlock()
		parent.mu.RUnlock()
	}
}

// markAsCompleted marks the ReadonlyAsyncResult as completed and signals any waiters.
// This is an internal method used by Set, SetResult, and SetError.
//
// The method ensures that:
// - The completion flag is set atomically
// - The completion channel is closed to signal any waiting goroutines
// - We wait for the background completion routine to finish
//
// This method is idempotent and will do nothing if the ReadonlyAsyncResult is already completed.
func (a *ReadonlyAsyncResult[T]) markAsCompleted() {
	if a.IsCompleted() {
		return
	}
	a.isCompleted.Store(true)
	close(a.completionCh)
	a.completionWg.Wait()
}

// IsCompleted returns true if the ReadonlyAsyncResult has been completed, either with
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
func (a *ReadonlyAsyncResult[T]) IsCompleted() bool {
	return a.isCompleted.Load()
}

// Result returns the result value and any error. This method blocks until
// the ReadonlyAsyncResult is completed, either by setting a value/error or by
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
func (a *ReadonlyAsyncResult[T]) Result() (T, error) {
	a.completionWg.Wait()
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.result, a.err
}

// Fork creates a new ReadonlyAsyncResult that will be completed with the same
// result and error as this one. This allows creating dependent operations
// that only proceed when the parent operation completes.
//
// The forked result inherits the same context as the parent, so it will
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
//	child := parent.Fork()
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
func (a *ReadonlyAsyncResult[T]) Fork() *ReadonlyAsyncResult[T] {
	res := NewReadonlyAsyncResult[T](a.ctx)
	go res.forkFrom(a)
	return res
}

// WritableAsyncResult extends ReadonlyAsyncResult with methods to set results and errors.
// It provides a clear separation between consumers and producers of async results.
//
// WritableAsyncResult adds the ability to:
// - Set successful result values with SetResult
// - Set error conditions with SetError
// - Set both result and error simultaneously with Set
// - Explicitly cancel operations with Cancel
type WritableAsyncResult[T any] struct {
	// cancel is the function to cancel the internal context
	cancel context.CancelFunc

	// ReadonlyAsyncResult contains the core implementation
	ReadonlyAsyncResult[T]
}

// NewWritableAsyncResult creates a new WritableAsyncResult instance with the given context.
// This is used when you need to both produce and consume an async result.
//
// The method creates a derived cancellable context from the provided parent context,
// allowing explicit cancellation through the Cancel method.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	result := NewWritableAsyncResult[string](ctx)
//
//	// Later, set the result
//	result.SetResult("computation complete")
func NewWritableAsyncResult[T any](ctx context.Context) *WritableAsyncResult[T] {
	cancelCtx, cancel := context.WithCancel(ctx)
	return &WritableAsyncResult[T]{
		cancel:              cancel,
		ReadonlyAsyncResult: *NewReadonlyAsyncResult[T](cancelCtx),
	}
}

// SetResult sets the result value and marks the ReadonlyAsyncResult as completed.
// If the ReadonlyAsyncResult is already completed, this method does nothing.
//
// This is a convenience method that calls Set with a nil error.
//
// Example:
//
//	result := NewWritableAsyncResult[string](ctx)
//	go func() {
//	    // Perform some computation
//	    time.Sleep(time.Second)
//	    result.SetResult("computation complete")
//	}()
//
//	// In another goroutine or later
//	value, err := result.Result() // Will block until result is set or ctx is canceled
func (a *WritableAsyncResult[T]) SetResult(result T) {
	a.Set(result, nil)
}

// SetError sets an error and marks the ReadonlyAsyncResult as completed with that error.
// If the ReadonlyAsyncResult is already completed, this method does nothing.
//
// This is a convenience method that calls Set with a zero value for the result type
// and the provided error.
//
// Example:
//
//	result := NewWritableAsyncResult[string](ctx)
//	go func() {
//	    // Attempt some operation
//	    if err := someRiskyOperation(); err != nil {
//	        result.SetError(err)
//	        return
//	    }
//	    result.SetResult("operation succeeded")
//	}()
func (a *WritableAsyncResult[T]) SetError(err error) {
	var zero T
	a.Set(zero, err)
}

// Set sets both the result value and error, then marks the ReadonlyAsyncResult as completed.
// If the ReadonlyAsyncResult is already completed, this method does nothing.
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
//	result := NewWritableAsyncResult[string](ctx)
//	go func() {
//	    res, err := processData(someData)
//	    result.Set(res, err) // Set both result and error at once
//	}()
func (a *WritableAsyncResult[T]) Set(result T, err error) {
	if a.IsCompleted() {
		return
	}
	a.mu.Lock()
	a.result = result
	a.err = err
	a.mu.Unlock()
	a.markAsCompleted()
}

// Cancel actively cancels the operation and marks the ReadonlyAsyncResult as completed
// with a context.Canceled error.
// If the ReadonlyAsyncResult is already completed, this method does nothing.
//
// This method provides an explicit way to cancel the operation beyond waiting
// for the parent context to be canceled. Calling this method immediately stops
// any work associated with this AsyncResult through the internal cancellable context.
//
// Example:
//
//	result := NewWritableAsyncResult[string](ctx)
//
//	// Later, if you need to cancel the operation:
//	result.Cancel()
//
//	// Anyone waiting on result.Result() will now receive context.Canceled error
func (a *WritableAsyncResult[T]) Cancel() {
	if a.IsCompleted() {
		return
	}
	a.cancel()
}
