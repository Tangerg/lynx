package flow

import (
	"context"
	"errors"
	"fmt"
)

// ParallelConfig contains the configuration for creating a Parallel node.
// It defines multiple nodes to execute concurrently and how to aggregate their results.
//
// Type parameters:
//   - I: Input type for all parallel nodes
//   - O: Output type after aggregation
type ParallelConfig[I any, O any] struct {
	// Nodes are the processing units to be executed in parallel.
	// Each node receives the same input.
	Nodes []Node[I, any]

	// Aggregator combines the outputs from successful nodes into a single result.
	// It receives only the outputs from nodes that completed successfully.
	Aggregator func(context.Context, []any) (O, error)

	// WaitCount specifies how many node completions to wait for before aggregating.
	// If <= 0, waits for all nodes to complete.
	// If > number of nodes, waits for all nodes.
	WaitCount int

	// RequiredSuccesses specifies the minimum number of successful completions needed.
	// If <= 0, requires all waited nodes to succeed.
	// If not met, returns an error even with ContinueOnError enabled.
	RequiredSuccesses int

	// ContinueOnError determines whether to continue waiting for other nodes when one fails.
	// If false, the first error cancels all remaining nodes and returns immediately.
	// If true, collects all errors and returns them together if RequiredSuccesses isn't met.
	ContinueOnError bool

	// CancelRemaining determines whether to cancel remaining nodes after WaitCount is reached.
	// Only applies when WaitCount < total nodes.
	CancelRemaining bool
}

// validate checks if the ParallelConfig is valid and ready to use.
// Returns an error if no nodes are provided or aggregator is missing.
func (cfg *ParallelConfig[I, O]) validate() error {
	if len(cfg.Nodes) == 0 {
		return errors.New("parallel must contain at least one node: no processing units defined")
	}

	if cfg.Aggregator == nil {
		return errors.New("parallel must have aggregator: function required to combine parallel results")
	}

	return nil
}

// Parallel represents a node that executes multiple nodes concurrently.
// It waits for a specified number of completions, validates success requirements,
// and aggregates the results into a single output.
//
// Type parameters:
//   - I: Input type for all parallel nodes
//   - O: Output type after aggregation
type Parallel[I any, O any] struct {
	nodes             []Node[I, any]
	aggregator        func(context.Context, []any) (O, error)
	waitCount         int
	requiredSuccesses int
	continueOnError   bool
	cancelRemaining   bool
}

// NewParallel creates a new Parallel instance with the provided configuration.
// Returns an error if the configuration is invalid.
//
// Example:
//
//	parallel, err := NewParallel(&ParallelConfig{
//	    Nodes: []Node[string, any]{node1, node2, node3},
//	    Aggregator: func(ctx context.Context, results []any) (string, error) {
//	        // Combine results
//	        return combineResults(results), nil
//	    },
//	    WaitCount: 2,          // Wait for first 2 completions
//	    RequiredSuccesses: 1,  // At least 1 must succeed
//	    ContinueOnError: true, // Don't stop on first error
//	})
func NewParallel[I any, O any](cfg *ParallelConfig[I, O]) (*Parallel[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Parallel[I, O]{
		nodes:             cfg.Nodes,
		aggregator:        cfg.Aggregator,
		waitCount:         cfg.WaitCount,
		requiredSuccesses: cfg.RequiredSuccesses,
		continueOnError:   cfg.ContinueOnError,
		cancelRemaining:   cfg.CancelRemaining,
	}, nil
}

// getWaitCount returns the effective number of nodes to wait for.
// Returns the total number of nodes if waitCount is invalid or exceeds total.
func (p *Parallel[I, O]) getWaitCount() int {
	if p.waitCount <= 0 {
		return len(p.nodes)
	}
	return min(p.waitCount, len(p.nodes))
}

// getRequiredSuccesses returns the effective number of required successful completions.
// Returns the wait count if requiredSuccesses is invalid or exceeds wait count.
func (p *Parallel[I, O]) getRequiredSuccesses() int {
	if p.requiredSuccesses <= 0 {
		return p.getWaitCount()
	}
	return min(p.requiredSuccesses, p.getWaitCount())
}

// nodeResult represents the result from a single node execution.
type nodeResult struct {
	output any
	err    error
}

// launchNodes starts all configured nodes in separate goroutines.
// Returns a channel for receiving results and a channel for signaling shutdown.
//
// Parameters:
//   - ctx: Context for cancellation control
//   - input: Input value for all nodes
//
// Returns:
//   - resultChan: Channel for receiving node results
//   - shutdownChan: Channel for signaling early termination to goroutines
func (p *Parallel[I, O]) launchNodes(ctx context.Context, input I) (<-chan *nodeResult, chan struct{}) {
	resultChan := make(chan *nodeResult, len(p.nodes))
	shutdownChan := make(chan struct{})

	for _, node := range p.nodes {
		n := node // Capture loop variable

		go func() {
			output, err := n.Run(ctx, input)

			select {
			case <-ctx.Done():
				// Context canceled, don't send result
				return
			case <-shutdownChan:
				// Shutdown signal received, don't send result
				return
			default:
				resultChan <- &nodeResult{
					output: output,
					err:    err,
				}
			}
		}()
	}

	return resultChan, shutdownChan
}

// validateResults checks if the collected results meet the required success count.
// Returns an error if insufficient successful results are collected.
//
// Parameters:
//   - successfulResults: Slice of successful outputs
//   - collectedErrors: Slice of errors from failed nodes
//
// Returns:
//   - The successful results if validation passes
//   - A combined error including all collected errors and a validation error
func (p *Parallel[I, O]) validateResults(successfulResults []any, collectedErrors []error) ([]any, error) {
	requiredCount := p.getRequiredSuccesses()

	if len(successfulResults) < requiredCount {
		validationErr := fmt.Errorf(
			"insufficient successful results: received %d out of %d required (total nodes: %d)",
			len(successfulResults),
			requiredCount,
			len(p.nodes),
		)

		allErrors := append(collectedErrors, validationErr)
		return nil, errors.Join(allErrors...)
	}

	return successfulResults, nil
}

// collectResults waits for and collects results from the launched nodes.
// Implements the configured waiting and error handling strategy.
//
// Parameters:
//   - ctx: Context for cancellation control
//   - resultChan: Channel for receiving node results
//   - cancel: Function to cancel the context for remaining nodes
//
// Returns:
//   - Slice of successful outputs
//   - An error if validation fails or a node fails with ContinueOnError=false
func (p *Parallel[I, O]) collectResults(
	ctx context.Context,
	resultChan <-chan *nodeResult,
	cancel context.CancelFunc,
) ([]any, error) {
	waitCount := p.getWaitCount()
	successfulResults := make([]any, 0, waitCount)
	collectedErrors := make([]error, 0)

	for range waitCount {
		select {
		case <-ctx.Done():
			// Context was canceled, stop collecting results
			return nil, ctx.Err()

		case result := <-resultChan:
			if result.err == nil {
				// Successful result
				successfulResults = append(successfulResults, result.output)
			} else if p.continueOnError {
				// Collect error and continue
				collectedErrors = append(collectedErrors, result.err)
			} else {
				// Fail fast: cancel remaining and return immediately
				cancel()
				return nil, result.err
			}
		}
	}

	// Cancel remaining nodes if configured
	if p.cancelRemaining {
		cancel()
	}

	return p.validateResults(successfulResults, collectedErrors)
}

// Run implements the Node interface for Parallel.
// It executes all configured nodes concurrently, waits for results according
// to the configuration, and aggregates the successful outputs.
//
// Execution flow:
//  1. Launch all nodes in separate goroutines
//  2. Collect results until WaitCount is reached or error occurs
//  3. Validate that RequiredSuccesses count is met
//  4. Aggregate successful results using the aggregator function
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for all parallel nodes
//
// Returns:
//   - The aggregated output from successful nodes
//   - An error if any node fails (when ContinueOnError=false),
//     if RequiredSuccesses isn't met, or if aggregation fails
func (p *Parallel[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O

	// Create a cancellable context for node execution
	nodeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Step 1: Launch all nodes
	resultChan, shutdownChan := p.launchNodes(nodeCtx, input)
	defer func() {
		close(shutdownChan)
	}()

	// Step 2: Collect and validate results
	successfulOutputs, err := p.collectResults(ctx, resultChan, cancel)
	if err != nil {
		return zero, err
	}

	// Step 3: Aggregate results
	return p.aggregator(ctx, successfulOutputs)
}
