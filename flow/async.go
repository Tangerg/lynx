package flow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/pkg/sync"
)

// AsyncConfig contains the configuration for creating an Async node.
// It defines the node to be executed asynchronously and the thread pool to use.
type AsyncConfig[I any, O any] struct {
	// Node is the processing unit to be executed asynchronously
	Node Node[I, O]

	// Pool is the thread pool for executing the async task.
	// If nil, a default no-pool executor will be used.
	Pool sync.Pool
}

// validate checks if the AsyncConfig is valid and applies defaults.
// Returns an error if the config or its Node field is nil.
// Sets a default Pool if none is provided.
func (cfg *AsyncConfig[I, O]) validate() error {
	if cfg == nil {
		return errors.New("async config cannot be nil")
	}

	if cfg.Node == nil {
		return errors.New("async node cannot be nil")
	}

	if cfg.Pool == nil {
		cfg.Pool = sync.PoolOfNoPool()
	}

	return nil
}

// Async represents a node that executes another node asynchronously.
// It returns a Future that can be used to retrieve the result later.
type Async[I any, O any] struct {
	node Node[I, O]
	pool sync.Pool
}

// NewAsync creates a new Async instance with the provided configuration.
// Returns an error if the configuration is invalid.
//
// Example:
//
//	async, err := NewAsync(&AsyncConfig{
//	    Node: myNode,
//	    Pool: myThreadPool,
//	})
func NewAsync[I any, O any](cfg *AsyncConfig[I, O]) (*Async[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Async[I, O]{
		node: cfg.Node,
		pool: cfg.Pool,
	}, nil
}

// RunType executes the configured node asynchronously and returns a typed Future.
// The Future can be used to retrieve the result when it's ready or check for errors.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for the async node
//
// Returns:
//   - A Future[O] that will eventually contain the result
//   - An error if the async task cannot be submitted
//
// Example:
//
//	future, err := async.RunType(ctx, input)
//	if err != nil {
//	    return err
//	}
//	result, err := future.Get()
func (a *Async[I, O]) RunType(ctx context.Context, input I) (sync.Future[O], error) {
	return sync.NewFutureTaskAndRunWithPool(
		func(interrupt <-chan struct{}) (output O, err error) {
			select {
			case <-interrupt:
				return output, fmt.Errorf("async task cancelled")
			default:
				return a.node.Run(ctx, input)
			}
		},
		a.pool,
	)
}

// Run implements the Node interface for Async.
// It executes the configured node asynchronously and returns a Future as any type.
// For type-safe usage, prefer using RunType instead.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for the async node
//
// Returns:
//   - A Future (as any type) that will eventually contain the result
//   - An error if the async task cannot be submitted
func (a *Async[I, O]) Run(ctx context.Context, input I) (any, error) {
	return a.RunType(ctx, input)
}
