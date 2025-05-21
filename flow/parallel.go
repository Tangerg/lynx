package flow

import (
	"context"
	"errors"
	"fmt"
)

// Parallel enables concurrent execution of multiple processors on the same input.
// It can wait for some or all processors to complete and aggregate their results.
// Generic parameters I and O define the input and output types for the parallel operation.
type Parallel[I any, O any] struct {
	// processors are the functions to execute in parallel
	processors []Processor[I, any]
	// waitCount is the number of processors to wait for (default: all)
	waitCount int
	// requiredSuccesses is the minimum number of successful results required (default: waitCount)
	requiredSuccesses int
	// continueOnError determines whether to continue waiting after an error
	continueOnError bool
	// cancelRemaining determines whether to cancel remaining processors when enough results are collected
	cancelRemaining bool
	// aggregator combines the results from multiple processors
	aggregator func(context.Context, []any) (O, error)
}

// validate ensures that the parallel operation has the necessary components.
// Returns an error if no processors are defined or aggregator is missing.
func (p *Parallel[I, O]) validate() error {
	if len(p.processors) == 0 {
		return errors.New("parallel must contain at least one processor: no processing functions defined")
	}
	if p.aggregator == nil {
		return errors.New("parallel must have aggregator: function required to combine parallel results")
	}
	return nil
}

// getWaitCount returns the number of processors to wait for.
// If waitCount is <= 0, it waits for all processors.
// Otherwise, it waits for min(waitCount, len(processors)).
func (p *Parallel[I, O]) getWaitCount() int {
	if p.waitCount <= 0 {
		return len(p.processors)
	}
	return min(p.waitCount, len(p.processors))
}

// getRequiredSuccesses returns the minimum number of successful results required.
// If requiredSuccesses is <= 0, it requires getWaitCount() successes.
// Otherwise, it requires min(requiredSuccesses, getWaitCount()).
func (p *Parallel[I, O]) getRequiredSuccesses() int {
	if p.requiredSuccesses <= 0 {
		return p.getWaitCount()
	}
	return min(p.requiredSuccesses, p.getWaitCount())
}

// parallelProcessResult holds the result of a single processor execution.
// It contains either the output value or an error.
type parallelProcessResult struct {
	output any
	error  error
}

// launchProcessors starts all processors in separate goroutines.
// Each processor's result is sent to resultChannel when complete.
// Returns channels for receiving results and signaling shutdown.
func (p *Parallel[I, O]) launchProcessors(ctx context.Context, input any) (chan *parallelProcessResult, chan struct{}) {
	resultChannel := make(chan *parallelProcessResult, len(p.processors))
	closeChannel := make(chan struct{}, 1)
	for _, processor := range p.processors {
		go func() {
			output, err := processor.Run(ctx, input)
			select {
			case <-ctx.Done():
				return
			case <-closeChannel:
				return
			default:
				resultChannel <- &parallelProcessResult{output, err}
			}
		}()
	}
	return resultChannel, closeChannel
}

// validateResults checks if enough successful results were collected.
// Returns the successful results if enough are available, otherwise returns an error.
func (p *Parallel[I, O]) validateResults(results []any, errs []error) ([]any, error) {
	if len(results) < p.getRequiredSuccesses() {
		errs = append(errs, fmt.Errorf("insufficient successful results: received %d out of %d required (total processors: %d)",
			len(results), p.getRequiredSuccesses(), len(p.processors)))
		return nil, errors.Join(errs...)
	}
	return results, nil
}

// collectResults waits for processor results up to waitCount.
// If continueOnError is false, it returns immediately on the first error.
// If cancelRemaining is true, it cancels the context after collecting enough results.
// Returns the collected results and any errors encountered.
func (p *Parallel[I, O]) collectResults(ctx context.Context, resultChannel <-chan *parallelProcessResult, cancel context.CancelFunc) ([]any, error) {
	waitCount := p.getWaitCount()
	results := make([]any, 0, waitCount)
	errs := make([]error, 0, waitCount)
	for range waitCount {
		select {
		case <-ctx.Done():
			// Context was canceled, stop collecting results
		case result := <-resultChannel:
			if result.error == nil {
				results = append(results, result.output)
			} else if p.continueOnError {
				errs = append(errs, result.error)
			} else {
				cancel()
				return nil, result.error
			}
		}
	}
	if p.cancelRemaining {
		cancel()
	}
	return p.validateResults(results, errs)
}

// aggregateResults combines the results from multiple processors.
// It first checks for context cancellation, then calls the aggregator function.
// Returns the aggregated result and any error encountered during aggregation.
func (p *Parallel[I, O]) aggregateResults(ctx context.Context, results []any) (res O, err error) {
	err = p.processors[0].checkContextCancellation(ctx)
	if err != nil {
		return
	}
	return p.aggregator(ctx, results)
}

// run executes the parallel operation.
// It launches processors, collects results, and aggregates them.
// Returns the aggregated output and any error encountered during execution.
func (p *Parallel[I, O]) run(ctx context.Context, input any) (o O, err error) {
	cancelCtx, cancel := context.WithCancel(ctx)
	resultChan, shutdownChan := p.launchProcessors(cancelCtx, input)
	defer func() { close(shutdownChan); close(resultChan) }()
	outputs, err := p.collectResults(ctx, resultChan, cancel)
	if err != nil {
		return
	}
	return p.aggregateResults(ctx, outputs)
}

// Run implements the Node interface for Parallel.
// It first validates the parallel configuration, then executes the parallel operation.
func (p *Parallel[I, O]) Run(ctx context.Context, input I) (o O, err error) {
	err = p.validate()
	if err != nil {
		return
	}
	return p.run(ctx, input)
}

// WithWaitCount sets the number of processors to wait for.
// A value <= 0 means wait for all processors.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithWaitCount(waitCount int) *Parallel[I, O] {
	p.waitCount = waitCount
	return p
}

// WithWaitAny configures the parallel operation to wait for any one processor to complete.
// Equivalent to WithWaitCount(1).
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithWaitAny() *Parallel[I, O] {
	p.WithWaitCount(1)
	return p
}

// WithWaitAll configures the parallel operation to wait for all processors to complete.
// Equivalent to WithWaitCount(-1).
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithWaitAll() *Parallel[I, O] {
	p.WithWaitCount(-1)
	return p
}

// AddProcessors adds one or more processors to execute in parallel.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) AddProcessors(processors ...Processor[I, any]) *Parallel[I, O] {
	p.processors = append(p.processors, processors...)
	return p
}

// WithAggregator sets the function that combines results from multiple processors.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithAggregator(aggregator func(context.Context, []any) (O, error)) *Parallel[I, O] {
	p.aggregator = aggregator
	return p
}

// WithCancelRemaining sets whether to cancel remaining processors after collecting enough results.
// If true, processors that haven't completed will be canceled when waitCount results are collected.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithCancelRemaining() *Parallel[I, O] {
	p.cancelRemaining = true
	return p
}

// WithContinueOnError sets whether to continue collecting results after an error.
// If true, errors are collected and returned after waitCount results or errors.
// If false (default), the operation stops on the first error.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithContinueOnError() *Parallel[I, O] {
	p.continueOnError = true
	return p
}

// WithRequiredSuccesses sets the minimum number of successful results required.
// If fewer than this number of processors succeed, the operation fails.
// Returns the Parallel for chaining.
func (p *Parallel[I, O]) WithRequiredSuccesses(requiredSuccesses int) *Parallel[I, O] {
	p.requiredSuccesses = requiredSuccesses
	return p
}

// ParallelBuilder helps construct a Parallel node with a fluent API.
// It maintains references to both the parent flow and the parallel operation being built.
type ParallelBuilder struct {
	flow     *Flow
	parallel *Parallel[any, any]
}

// WithWaitCount sets the number of processors to wait for.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithWaitCount(waitCount int) *ParallelBuilder {
	p.parallel.WithWaitCount(waitCount)
	return p
}

// WithWaitAny configures the parallel operation to wait for any one processor to complete.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithWaitAny() *ParallelBuilder {
	p.parallel.WithWaitAny()
	return p
}

// WithWaitAll configures the parallel operation to wait for all processors to complete.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithWaitAll() *ParallelBuilder {
	p.parallel.WithWaitAll()
	return p
}

// AddProcessors adds one or more processors to execute in parallel.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) AddProcessors(processors ...Processor[any, any]) *ParallelBuilder {
	p.parallel.AddProcessors(processors...)
	return p
}

// WithAggregator sets the function that combines results from multiple processors.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *ParallelBuilder {
	p.parallel.WithAggregator(aggregator)
	return p
}

// WithCancelRemaining sets whether to cancel remaining processors after collecting enough results.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithCancelRemaining() *ParallelBuilder {
	p.parallel.WithCancelRemaining()
	return p
}

// WithContinueOnError sets whether to continue collecting results after an error.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithContinueOnError() *ParallelBuilder {
	p.parallel.WithContinueOnError()
	return p
}

// WithRequiredSuccesses sets the minimum number of successful results required.
// Returns the ParallelBuilder for chaining.
func (p *ParallelBuilder) WithRequiredSuccesses(requiredSuccesses int) *ParallelBuilder {
	p.parallel.WithRequiredSuccesses(requiredSuccesses)
	return p
}

// Then adds the constructed parallel operation to the parent flow and returns the flow.
// This completes the parallel construction and continues building the flow.
func (p *ParallelBuilder) Then() *Flow {
	p.flow.Then(p.parallel)
	return p.flow
}
