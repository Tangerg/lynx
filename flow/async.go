package flow

import (
	"context"

	"github.com/Tangerg/lynx/pkg/sync"
)

// Async enables asynchronous execution of a processor.
// Instead of waiting for the processor to complete, it returns a sync.Future
// that can be used to check completion status and retrieve the result later.
// Generic parameters I and O define the input and output types for the processor.
type Async[I any, O any] struct {
	processor Processor[I, O]
	pool      sync.Pool
}

// getPool returns the pool to be used for async execution.
// If a pool has been explicitly set, it returns that pool.
// Otherwise, it returns the default pool from sync package.
func (a *Async[I, O]) getPool() sync.Pool {
	if a.pool != nil {
		return a.pool
	}
	return sync.DefaultPool()
}

// RunType executes the processor asynchronously and returns a typed sync.Future.
// It validates the processor, then uses the pool to run it in a separate goroutine.
// Returns the future handle and any error encountered during setup.
func (a *Async[I, O]) RunType(ctx context.Context, input I) (sync.Future[O], error) {
	err := validateProcessor(a.processor)
	if err != nil {
		return nil, err
	}
	return sync.NewFutureTaskAndRunWithPool(
		func(interrupt <-chan struct{}) (O, error) {
			return a.processor.Run(ctx, input)
		},
		a.getPool(),
	)
}

// Run implements the Node interface for Async.
// It executes the processor asynchronously and returns the sync.Future as an any type.
func (a *Async[I, O]) Run(ctx context.Context, input I) (any, error) {
	return a.RunType(ctx, input)
}

// WithProcessor sets the processor to execute asynchronously.
// Returns the Async for chaining.
func (a *Async[I, O]) WithProcessor(processor Processor[I, O]) *Async[I, O] {
	a.processor = processor
	return a
}

// WithPool sets the goroutine pool to use for async execution.
// Returns the Async for chaining.
func (a *Async[I, O]) WithPool(pool sync.Pool) *Async[I, O] {
	a.pool = pool
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

// WithPool sets the goroutine pool to use for async execution.
// Returns the AsyncBuilder for chaining.
func (a *AsyncBuilder) WithPool(pool sync.Pool) *AsyncBuilder {
	a.async.WithPool(pool)
	return a
}

// Then adds the constructed async operation to the parent flow and returns the flow.
// This completes the async construction and continues building the flow.
func (a *AsyncBuilder) Then() *Flow {
	a.flow.Then(a.async)
	return a.flow
}
