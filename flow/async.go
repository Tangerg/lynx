package flow

import (
	"context"
	"sync"
	"sync/atomic"
)

// ReadonlyAsyncResult represents the eventual result of an asynchronous operation.
// It provides read-only access to the completion status and result.
// Generic parameter T defines the type of the result.
type ReadonlyAsyncResult[T any] struct {
	// ctx is the context associated with this result
	ctx context.Context
	// result holds the value when the operation completes successfully
	result T
	// error holds any error that occurred during the operation
	error error
	// isCompleted is an atomic flag indicating whether the operation has completed
	isCompleted atomic.Bool
	// completionCh is closed when the operation completes
	completionCh chan struct{}
	// completionWg is used to ensure cleanup tasks complete before accessing results
	completionWg sync.WaitGroup
}

// NewReadonlyAsyncResult creates a new ReadonlyAsyncResult bound to the given context.
// It initializes the completion channel and starts a goroutine to monitor context cancellation.
func NewReadonlyAsyncResult[T any](ctx context.Context) *ReadonlyAsyncResult[T] {
	res := &ReadonlyAsyncResult[T]{ctx: ctx, completionCh: make(chan struct{}, 1)}
	res.completionWg.Add(1)
	go res.awaitCompletion()
	return res
}

// awaitCompletion monitors the context for cancellation.
// If the context is canceled before the operation completes, it marks the result as completed with the context error.
func (a *ReadonlyAsyncResult[T]) awaitCompletion() {
	defer a.completionWg.Done()
	select {
	case <-a.ctx.Done():
		if a.isCompleted.CompareAndSwap(false, true) {
			a.error = a.ctx.Err()
			close(a.completionCh)
		}
	case <-a.completionCh:
		// Operation completed normally, nothing to do
	}
}

// forkFrom copies the completion status, result, and error from a parent result when it completes.
// This allows creating multiple independent handles to the same asynchronous operation.
func (a *ReadonlyAsyncResult[T]) forkFrom(parent *ReadonlyAsyncResult[T]) {
	select {
	case <-parent.completionCh:
		if a.isCompleted.CompareAndSwap(false, true) {
			a.result, a.error = parent.result, parent.error
			close(a.completionCh)
		}
	}
}

// IsCompleted returns true if the operation has completed (successfully or with an error).
// It uses atomic operations to safely check the completion status.
func (a *ReadonlyAsyncResult[T]) IsCompleted() bool { return a.isCompleted.Load() }

// Result blocks until the operation completes and returns the result and any error.
// It waits for the completion goroutine to finish before accessing the result and error.
func (a *ReadonlyAsyncResult[T]) Result() (T, error) {
	a.completionWg.Wait()
	return a.result, a.error
}

// Fork creates a new ReadonlyAsyncResult that will have the same result as this one.
// This allows multiple callers to independently wait for and access the same result.
func (a *ReadonlyAsyncResult[T]) Fork() *ReadonlyAsyncResult[T] {
	res := NewReadonlyAsyncResult[T](a.ctx)
	go res.forkFrom(a)
	return res
}

// WritableAsyncResult extends ReadonlyAsyncResult with the ability to set the result.
// It's used internally to complete asynchronous operations.
// Generic parameter T defines the type of the result.
type WritableAsyncResult[T any] struct {
	// cancel is a function that can cancel the associated context
	cancel context.CancelFunc
	// embedded ReadonlyAsyncResult provides the read-only functionality
	ReadonlyAsyncResult[T]
}

// NewWritableAsyncResult creates a new WritableAsyncResult with a cancellable context.
// The cancellable context allows explicitly canceling the operation if needed.
func NewWritableAsyncResult[T any](ctx context.Context) *WritableAsyncResult[T] {
	cancelCtx, cancel := context.WithCancel(ctx)
	return &WritableAsyncResult[T]{cancel: cancel, ReadonlyAsyncResult: *NewReadonlyAsyncResult[T](cancelCtx)}
}

// SetResult sets a successful result for the operation.
// This marks the operation as completed with no error.
func (a *WritableAsyncResult[T]) SetResult(result T) { a.Set(result, nil) }

// SetError sets an error result for the operation.
// This marks the operation as completed with the given error.
func (a *WritableAsyncResult[T]) SetError(err error) { var zero T; a.Set(zero, err) }

// Set sets both the result and error for the operation.
// This marks the operation as completed and notifies any waiting goroutines.
func (a *WritableAsyncResult[T]) Set(result T, err error) {
	if a.isCompleted.CompareAndSwap(false, true) {
		a.result, a.error = result, err
		close(a.completionCh)
	}
}

// Cancel explicitly cancels the operation if it hasn't completed.
// This triggers context cancellation for any code using the operation's context.
func (a *WritableAsyncResult[T]) Cancel() {
	if a.IsCompleted() {
		return
	}
	a.cancel()
}

// Async enables asynchronous execution of a processor.
// Instead of waiting for the processor to complete, it returns a ReadonlyAsyncResult
// that can be used to check completion status and retrieve the result later.
// Generic parameters I and O define the input and output types for the processor.
type Async[I any, O any] struct {
	// processor is the function to execute asynchronously
	processor Processor[I, O]
}

// RunType executes the processor asynchronously and returns a typed ReadonlyAsyncResult.
// It validates the processor, then starts a goroutine to run it and set the result.
// Returns the result handle and any error encountered during setup.
func (a *Async[I, O]) RunType(ctx context.Context, input I) (*ReadonlyAsyncResult[O], error) {
	err := validateProcessor(a.processor)
	if err != nil {
		return nil, err
	}
	res := NewWritableAsyncResult[O](ctx)
	go func() { output, err1 := a.processor.Run(ctx, input); res.Set(output, err1) }()
	return res.Fork(), nil
}

// Run implements the Node interface for Async.
// It executes the processor asynchronously and returns the result handle as an any type.
func (a *Async[I, O]) Run(ctx context.Context, input I) (any, error) {
	return a.RunType(ctx, input)
}

// WithProcessor sets the processor to execute asynchronously.
// Returns the Async for chaining.
func (a *Async[I, O]) WithProcessor(processor Processor[I, O]) *Async[I, O] {
	a.processor = processor
	return a
}

// AsyncBuilder helps construct an Async node with a fluent API.
// It maintains references to both the parent flow and the async operation being built.
type AsyncBuilder struct {
	flow  *Flow
	async *Async[any, any]
}

// WithProcessor sets the processor to execute asynchronously.
// Returns the AsyncBuilder for chaining.
func (a *AsyncBuilder) WithProcessor(processor Processor[any, any]) *AsyncBuilder {
	a.async.WithProcessor(processor)
	return a
}

// Then adds the constructed async operation to the parent flow and returns the flow.
// This completes the async construction and continues building the flow.
func (a *AsyncBuilder) Then() *Flow {
	a.flow.Then(a.async)
	return a.flow
}
