package flow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Future represents an asynchronous computation result that may not be immediately available.
// It provides multiple methods to retrieve the result with different blocking strategies.
//
// Note: This package does not provide a default Future implementation.
// Users must implement this interface according to their specific needs.
type Future[V any] interface {
	// Get blocks until the result is available or an error occurs.
	Get() (V, error)

	// GetWithTimeout blocks until the result is available, the timeout expires,
	// or an error occurs. Returns an error if the timeout is exceeded.
	GetWithTimeout(timeout time.Duration) (V, error)

	// GetWithContext blocks until the result is available, the context is cancelled,
	// or an error occurs. Returns an error if the context is cancelled.
	GetWithContext(ctx context.Context) (V, error)

	// TryGet attempts to retrieve the result without blocking.
	// Returns:
	//   - value: the result if available
	//   - error: any error that occurred during computation
	//   - ready: true if the result is ready, false otherwise
	TryGet() (value V, err error, ready bool)
}

var _ Node[any, Future[any]] = (*Async[any, any, Future[any]])(nil)

// Async represents a node that executes asynchronous operations and returns a Future
// for deferred result retrieval.
//
// This node is useful for:
//   - Long-running operations that shouldn't block the workflow
//   - Operations that can be executed in the background
//   - Computations where results are needed at a later time
//
// The actual Future implementation must be provided by the user, allowing flexibility
// in choosing or implementing different asynchronous execution strategies.
type Async[I, O any, F Future[O]] struct {
	processor func(context.Context, I) (F, error)
}

// NewAsync creates a new async node with the provided processor function.
//
// The processor should:
//   - Accept an input and return a Future that will eventually contain the output
//   - Handle its own asynchronous execution logic
//   - Return an error if the async operation cannot be started
//
// Returns an error if the processor is nil.
func NewAsync[I, O any, F Future[O]](processor func(context.Context, I) (F, error)) (*Async[I, O, F], error) {
	if processor == nil {
		return nil, errors.New("async processor cannot be nil")
	}

	return &Async[I, O, F]{
		processor: processor,
	}, nil
}

// Run executes the async processor and returns a Future for the result.
//
// The returned Future allows the caller to retrieve the result at a later time
// using one of the Future's retrieval methods (Get, GetWithTimeout, etc.).
//
// Note: This method returns immediately after starting the async operation.
// The actual computation happens asynchronously, and results are obtained
// through the returned Future.
func (a *Async[I, O, F]) Run(ctx context.Context, input I) (F, error) {
	future, err := a.processor(ctx, input)
	if err != nil {
		var zero F
		return zero, fmt.Errorf("failed to start async operation: %w", err)
	}

	return future, nil
}
