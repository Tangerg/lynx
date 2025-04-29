package flow

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
)

// Node is the fundamental interface that all flow processing nodes implement.
//
// The Node interface represents an individual processing unit in a flow pipeline,
// encapsulating specific processing logic that transforms inputs into outputs.
// Nodes can be connected together to form complex processing workflows.
//
// All Node implementations should properly handle context cancellation,
// ensuring that long-running operations can be gracefully terminated.
type Node[I any, O any] interface {
	// Run executes the node's processing logic with the provided input and context.
	//
	// The input parameter can be any type, allowing for flexible data processing.
	// The context parameter provides cancellation signals and timeouts.
	//
	// Returns the processed output and any error that occurred during processing.
	// If the context is canceled, implementations should return ctx.Err().
	Run(ctx context.Context, input I) (O, error)
}

// core provides the base implementation and common functionality for all node types.
//
// The core struct is not meant to be used directly but serves as an internal
// foundation for specialized node implementations. It provides essential mechanisms
// for context handling and processing function management.
type core[I any, O any] struct {
	// processor is the function that performs the actual data transformation
	// for this node. It takes a context and input value, and returns a processed
	// output and any error encountered.
	processor Processor[I, O]
}

// ensureContextActive checks if the context has been canceled or exceeded its deadline.
//
// This method is used internally to respect context cancellation in all node operations,
// ensuring proper shutdown behavior when pipelines are terminated early.
//
// Returns ctx.Err() if the context is done, otherwise nil.
func (n *core[I, O]) ensureContextActive(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// process executes the node's processor function with appropriate context handling.
//
// This method performs validation, checks context status, and runs the processing function.
// It's a central component used by all node implementations to execute their logic.
//
// Returns an error if:
//   - No processor function is defined
//   - The context has been canceled
//   - The processor function itself returns an error
func (n *core[I, O]) process(ctx context.Context, input I) (O, error) {
	var zero O
	if n.processor == nil {
		return zero, errors.New("processor function is required")
	}
	err := n.ensureContextActive(ctx)
	if err != nil {
		return zero, err
	}
	return n.processor(ctx, input)
}

// withProcessor assigns a processing function to the node.
//
// The processing function defines the actual transformation logic for this node.
// This is an internal method used by the public WithProcessor methods of concrete node types.
func (n *core[I, O]) withProcessor(processor Processor[I, O]) {
	if processor != nil {
		n.processor = processor
	}
}

// StepNode implements a simple single-step processing node in a flow pipeline.
//
// StepNode is the most basic node type that applies a single processing function
// to transform input into output. It forms the building block for more complex
// flow patterns.
type StepNode[I any, O any] struct {
	core[I, O]
}

// WithProcessor assigns a processing function to this node.
//
// The processor function defines the transformation that will be applied
// when the node is executed. It should take an input of type I and return
// an output of type O.
//
// Returns the StepNode instance for method chaining.
func (s *StepNode[I, O]) WithProcessor(processor Processor[I, O]) *StepNode[I, O] {
	s.withProcessor(processor)
	return s
}

// Run executes the node's processing logic with the provided input and context.
//
// This method applies the configured processor function to the input and
// returns the resulting output, handling context cancellation appropriately.
//
// If the processor is not configured or the context is canceled, an error is returned.
func (s *StepNode[I, O]) Run(ctx context.Context, input I) (O, error) {
	return s.process(ctx, input)
}

// BranchNode implements conditional routing of data based on the processing outcome.
//
// BranchNode enables decision-making within a workflow, allowing the pipeline to
// follow different paths based on data values or processing results. This is essential
// for implementing business logic with conditional processing.
//
// A BranchNode uses a route selector function to determine which branch to follow
// after processing. Each potential branch is identified by a unique string key.
//
// Example:
//
//	// Create a branch node that routes messages by priority
//	priorityRouter := &flow.BranchNode{}
//	    .WithProcessor(decodeMessage)
//	    .WithRouteSelector(func(ctx context.Context, input, output any) (string, error) {
//	        msg := output.(Message)
//	        return msg.Priority, nil
//	    })
//	    .AddBranch("high", highPriorityHandler)
//	    .AddBranch("normal", normalPriorityHandler)
//	    .AddBranch("low", lowPriorityHandler)
type BranchNode struct {
	core[any, any]
	// routeResolver determines which branch to follow based on input and output
	routeResolver func(context.Context, any, any) (string, error)
	// branches maps route identifiers to their corresponding Node implementations
	branches map[string]Node[any, any]
}

// resolveRoute determines which branch to follow based on input and output.
//
// This method uses the configured route selector function to choose a branch
// identifier, then returns the corresponding Node from the branches map.
//
// Returns:
//   - The selected Node to execute next
//   - An error if the route selection fails or if the selected branch doesn't exist
//   - nil, nil if no route selector is defined (indicating no branching)
func (b *BranchNode) resolveRoute(ctx context.Context, input any, output any) (Node[any, any], error) {
	if b.routeResolver == nil {
		return nil, errors.New("branch resolver is required")
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return nil, err
	}
	route, err := b.routeResolver(ctx, input, output)
	if err != nil {
		return nil, err
	}
	branch, ok := b.branches[route]
	if !ok {
		return nil, fmt.Errorf("branch route %s not found", route)
	}
	return branch, nil
}

// Run processes the input and routes to the appropriate branch.
//
// The method first applies this node's processing function to the input.
// It then uses the route selector to determine which branch to follow
// and delegates processing to that branch.
//
// If no branches are defined or no route selector is set, returns the
// output of this node's processor directly.
func (b *BranchNode) Run(ctx context.Context, input any) (any, error) {
	output, err := b.process(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(b.branches) == 0 || b.routeResolver == nil {
		return output, nil
	}

	branch, err := b.resolveRoute(ctx, input, output)
	if err != nil {
		return nil, err
	}
	return branch.Run(ctx, output)
}

// WithRouteResolver assigns a function that determines the branch to follow.
//
// The route selector examines the input and output of this node's processor
// and returns a string identifier for the branch to follow. This string must
// match one of the branch keys added with AddBranch.
//
// Returns the BranchNode instance for method chaining.
func (b *BranchNode) WithRouteResolver(routeResolver func(context.Context, any, any) (string, error)) *BranchNode {
	if routeResolver != nil {
		b.routeResolver = routeResolver
	}
	return b
}

// WithProcessor assigns a processing function to this node.
//
// The processor function transforms the input before branching decisions are made.
// The output of this processor will be passed to the route selector along with
// the original input to determine which branch to follow.
//
// Returns the BranchNode instance for method chaining.
func (b *BranchNode) WithProcessor(processor Processor[any, any]) *BranchNode {
	b.withProcessor(processor)
	return b
}

// AddBranch adds a new branch node with the given identifier.
//
// The route string is used as the key for selecting this branch when the
// route selector returns a matching identifier. Multiple branches can be
// added to create a comprehensive conditional processing system.
//
// Returns the BranchNode instance for method chaining.
func (b *BranchNode) AddBranch(route string, node Node[any, any]) *BranchNode {
	if b.branches == nil {
		b.branches = make(map[string]Node[any, any])
	}
	if node != nil {
		b.branches[route] = node
	}
	return b
}

// LoopNode implements iterative processing until a termination condition is met.
//
// LoopNode enables repetitive execution of the same processing logic,
// useful for scenarios like retry mechanisms, iterative algorithms, or
// processing until a specific condition is reached.
//
// A LoopNode continues processing until either:
//   - The stop condition returns true
//   - The processor returns an error
//   - The context is canceled
//
// Example:
//
//	// Create a loop that retries a network operation with backoff
//	retryNode := &flow.LoopNode{}
//	    .WithProcessor(makeNetworkRequest)
//	    .WithStopCondition(func(ctx context.Context, iteration int, input, output any) (bool, error) {
//	        // Stop if successful or after 5 attempts
//	        return output != nil || iteration >= 5, nil
//	    })
type LoopNode[I any, O any] struct {
	core[I, O]
	// terminator determines when to stop looping based on iteration count, input, and output
	terminator func(context.Context, int, I, O) (bool, error)
}

// shouldTerminate evaluates whether the looping should stop.
//
// This method checks the stop condition using the current state:
//   - iteration count: how many times the loop has executed
//   - input: the original input to the loop
//   - output: the result from the most recent iteration
//
// Returns:
//   - true if looping should stop, false to continue
//   - an error if condition evaluation fails
//   - true, nil if no stop condition is defined (to prevent infinite loops)
func (l *LoopNode[I, O]) shouldTerminate(ctx context.Context, iteration int, input I, output O) (bool, error) {
	if l.terminator == nil {
		return true, nil
	}
	err := l.ensureContextActive(ctx)
	if err != nil {
		return false, err
	}
	return l.terminator(ctx, iteration, input, output)
}

// runIteration performs recursive loop execution with iteration tracking.
//
// This internal method handles the actual looping mechanism, maintaining
// the iteration count and evaluating the termination condition after each run.
//
// If the stop condition returns true or an error occurs, the loop terminates.
// Otherwise, it recursively calls itself with an incremented iteration count.
func (l *LoopNode[I, O]) runIteration(ctx context.Context, iteration int, input I) (O, error) {
	var zero O
	output, err := l.process(ctx, input)
	if err != nil {
		return zero, err
	}
	shouldTerminate, err := l.shouldTerminate(ctx, iteration, input, output)
	if err != nil {
		return zero, err
	}
	if shouldTerminate {
		return output, nil
	}
	return l.runIteration(ctx, iteration+1, input)
}

// Run executes the processing logic in a loop until termination.
//
// This method initiates the looping process by calling runIteration
// with an initial iteration count of 0. The final output (after all iterations)
// is returned when the loop terminates.
func (l *LoopNode[I, O]) Run(ctx context.Context, input I) (O, error) {
	return l.runIteration(ctx, 0, input)
}

// WithProcessor assigns a processing function to this node.
//
// The processor function will be executed repeatedly in each iteration of the loop.
// Its output from one iteration becomes its input for the next iteration.
//
// Returns the LoopNode instance for method chaining.
func (l *LoopNode[I, O]) WithProcessor(processor Processor[I, O]) *LoopNode[I, O] {
	l.withProcessor(processor)
	return l
}

// WithTerminator assigns a function that determines when to stop looping.
//
// The terminator function examines:
//   - The current iteration count
//   - The original input to the loop
//   - The output from the most recent iteration
//
// It should return true when looping should stop, or false to continue.
//
// Returns the LoopNode instance for method chaining.
func (l *LoopNode[I, O]) WithTerminator(terminator func(context.Context, int, I, O) (bool, error)) *LoopNode[I, O] {
	if terminator != nil {
		l.terminator = terminator
	}
	return l
}

// BatchNode processes data in separate chunks or segments.
//
// BatchNode enables dividing a large input into smaller pieces for processing,
// then combining the results back together. This is useful for:
//   - Processing collections of items
//   - Chunking large datasets into manageable pieces
//   - Processing structured data with multiple components
//
// The BatchNode uses three main functions:
//   - A segmenter to divide input into chunks
//   - A processor to handle each individual chunk
//   - An aggregator to combine the processed results
//
// Example:
//
//	// Create a batch processor that processes each item in a slice
//	batchProcessor := &flow.BatchNode{}
//	    .WithProcessor(processItem)
//	    .WithAggregator(func(ctx context.Context, results []any) (any, error) {
//	        // Combine individual results into a summary
//	        return summarizeResults(results), nil
//	    })
type BatchNode[I any, O any, T any, R any] struct {
	core[T, R]
	allowFailure bool
	// segmenter divides the input into multiple segments for processing
	segmenter func(context.Context, I) ([]T, error)
	// aggregator combines processed segments back into a single result
	aggregator func(context.Context, []R) (O, error)
}

// createSegments divides the input into multiple chunks for processing.
//
// This method applies the segmenter function to divide the input into
// multiple segments that will be processed individually.
//
// If no segmenter is defined, returns an empty slice.
// Returns any error encountered during segmentation.
func (b *BatchNode[I, O, T, R]) createSegments(ctx context.Context, input I) ([]T, error) {
	if b.segmenter == nil {
		return make([]T, 0), nil
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return nil, err
	}
	return b.segmenter(ctx, input)
}

// aggregateResults combines the processed outputs into a single result.
//
// This method applies the aggregator function to combine the processed segment
// results into a final output.
//
// If no aggregator is defined, returns the zero value for type O.
// Returns any error encountered during aggregation.
func (b *BatchNode[I, O, T, R]) aggregateResults(ctx context.Context, results []R) (O, error) {
	var zero O
	if b.aggregator == nil {
		return zero, nil
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return zero, err
	}
	return b.aggregator(ctx, results)
}

// Run processes each segment individually and combines the results.
//
// The method:
//  1. Divides the input into segments
//  2. Processes each segment with the processor function
//  3. Collects results from successful operations
//  4. Aggregates the results into a final output
//
// If allowFailure is false, stops processing and returns the error on first failure.
// If allowFailure is true, continues processing all segments and returns the
// aggregated results from successful operations.
func (b *BatchNode[I, O, T, R]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	segments, err := b.createSegments(ctx, input)
	if err != nil {
		return zero, err
	}

	results := make([]R, 0, len(segments))

	for _, segment := range segments {
		output, err1 := b.process(ctx, segment)
		if err1 == nil {
			results = append(results, output)
		} else if !b.allowFailure {
			return zero, err1
		}
	}

	return b.aggregateResults(ctx, results)
}

// WithSegmenter assigns a function that divides input into multiple chunks.
//
// The segmenter function takes a single input and returns a slice of items
// to be processed individually. This is useful for splitting collections
// or complex data structures into their components.
//
// Returns the BatchNode instance for method chaining.
func (b *BatchNode[I, O, T, R]) WithSegmenter(segmenter func(context.Context, I) ([]T, error)) *BatchNode[I, O, T, R] {
	if segmenter != nil {
		b.segmenter = segmenter
	}
	return b
}

// WithAggregator assigns a function that combines processed results.
//
// The aggregator function takes a slice of individually processed results
// and combines them into a single output. This allows for summarization,
// merging, or other collective operations on the batch results.
//
// Returns the BatchNode instance for method chaining.
func (b *BatchNode[I, O, T, R]) WithAggregator(aggregator func(context.Context, []R) (O, error)) *BatchNode[I, O, T, R] {
	if aggregator != nil {
		b.aggregator = aggregator
	}
	return b
}

// WithProcessor assigns a processing function to this node.
//
// The processor function will be applied to each individual segment
// during batch processing. It should handle a single segment and
// return the processed result.
//
// Returns the BatchNode instance for method chaining.
func (b *BatchNode[I, O, T, R]) WithProcessor(processor Processor[T, R]) *BatchNode[I, O, T, R] {
	b.withProcessor(processor)
	return b
}

// AllowFailure configures whether the batch processing should continue
// when individual segment processing fails.
//
// When set to true, processing continues even if some segments fail.
// When false (default), processing stops on the first error.
//
// Returns the BatchNode instance for method chaining.
func (b *BatchNode[I, O, T, R]) AllowFailure(allowFailure bool) *BatchNode[I, O, T, R] {
	b.allowFailure = allowFailure
	return b
}

// ParallelNode processes input segments concurrently for improved performance.
//
// ParallelNode extends BatchNode to execute segment processing concurrently
// using goroutines. This can significantly improve performance for processing
// that doesn't require sequential execution, especially for:
//   - CPU-bound processing on multi-core systems
//   - IO-bound operations that can operate independently
//   - Any batch processing where items don't depend on each other
//
// The number of concurrent goroutines is determined by the limit parameter,
// or by the number of segments if no limit is set.
//
// Example:
//
//	// Create a parallel processor that downloads multiple files simultaneously
//	downloader := &flow.ParallelNode{}
//	    .WithProcessor(downloadFile)
//	    .WithAggregator(func(ctx context.Context, results []any) (any, error) {
//	        // Combine download results into a report
//	        return createDownloadReport(results), nil
//	    })
type ParallelNode[I any, O any, T any, R any] struct {
	BatchNode[I, O, T, R]
	limit int
}

// getLimit returns the configured concurrency limit or -1 if no limit is set.
//
// A negative return value indicates that the number of goroutines should
// only be limited by the number of segments to process.
func (p *ParallelNode[I, O, T, R]) getLimit() int {
	if p.limit <= 0 {
		return -1 // no limit
	}
	return p.limit
}

// Run processes each segment concurrently and combines the results.
//
// This method overrides BatchNode.Run to use goroutines for concurrent execution:
//  1. Divides the input into segments
//  2. Launches a goroutine for each segment (with optional limit)
//  3. Collects results as they complete
//  4. Waits for all processing to finish
//  5. Aggregates successful results
//
// Uses errgroup to coordinate goroutines and safely collect results
// from concurrent operations.
func (p *ParallelNode[I, O, T, R]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	segments, err := p.createSegments(ctx, input)
	if err != nil {
		return zero, err
	}

	var (
		channel         = make(chan R, len(segments))
		results         = make([]R, 0, len(segments))
		group, groupCtx = errgroup.WithContext(ctx)
	)

	group.SetLimit(p.getLimit())
	for _, segment := range segments {
		group.Go(func() error {
			output, err1 := p.process(groupCtx, segment)
			if err1 == nil {
				channel <- output
			}
			if !p.allowFailure {
				return err1
			}
			return nil
		})
	}

	err = group.Wait()
	if err != nil {
		return zero, err
	}

	close(channel)
	for result := range channel {
		results = append(results, result)
	}

	return p.aggregateResults(ctx, results)
}

// WithProcessor assigns a processing function to this node.
//
// The processor function will be applied to each segment concurrently.
// It should be thread-safe as it will be executed in separate goroutines.
//
// Returns the ParallelNode instance for method chaining.
func (p *ParallelNode[I, O, T, R]) WithProcessor(processor Processor[T, R]) *ParallelNode[I, O, T, R] {
	p.withProcessor(processor)
	return p
}

// AllowFailure configures whether the parallel processing should continue
// when individual segment processing fails.
//
// When set to true, processing continues even if some segments fail.
// When false (default), processing stops on the first error.
//
// Returns the ParallelNode instance for method chaining.
func (p *ParallelNode[I, O, T, R]) AllowFailure(allowFailure bool) *ParallelNode[I, O, T, R] {
	p.allowFailure = allowFailure
	return p
}

// Limit sets the maximum number of concurrent goroutines to use for processing.
//
// This allows control over resource utilization when processing large batches.
// A value <= 0 means no limit (use a goroutine for each segment).
//
// Returns the ParallelNode instance for method chaining.
func (p *ParallelNode[I, O, T, R]) Limit(limit int) *ParallelNode[I, O, T, R] {
	p.limit = limit
	return p
}

// AsyncNode executes processing in a non-blocking, asynchronous manner.
//
// AsyncNode allows for "fire and forget" processing where the caller doesn't
// need to wait for the operation to complete. It returns a ReadonlyAsyncResult that
// provides a structured way to retrieve results later, allowing the caller to:
//   - Continue execution without waiting for results
//   - Safely retrieve results when needed with proper cancellation handling
//   - Implement fan-out processing patterns with full type safety
//
// The ReadonlyAsyncResult returned by Run provides a concurrency-safe way to access
// the operation's result or error when processing completes, without the ability
// to modify the result.
//
// Example:
//
//	// Create an async node for background processing
//	backgroundProcessor := &flow.AsyncNode{}
//	    .WithProcessor(generateReport)
//
//	// Start processing and continue without waiting
//	resultAsync, _ := backgroundProcessor.Run(ctx, data)
//
//	// Optionally, retrieve results later
//	go func() {
//	    value, err := resultAsync.Result() // Blocks until result is available
//	    if err != nil {
//	        // Handle error
//	        return
//	    }
//	    // Use the result value
//	}()
//
//	// Or check if it's completed without blocking
//	if resultAsync.IsCompleted() {
//	    value, err := resultAsync.Result() // Won't block if completed
//	    // Handle result
//	}
type AsyncNode[I any, O any] struct {
	core[I, O]
}

// RunType executes processing asynchronously and returns a ReadonlyAsyncResult for the operation.
//
// This method launches a goroutine to perform the processing and immediately
// returns a ReadonlyAsyncResult that will be completed when processing finishes.
// The returned value is intentionally read-only to ensure the result can only
// be set by the async operation itself.
//
// The returned ReadonlyAsyncResult:
//   - Provides thread-safe access to the operation's result or error
//   - Integrates with the provided context for cancellation handling
//   - Can be checked for completion status without blocking
//   - Guarantees eventual completion (either with result, error, or context cancellation)
//   - Prevents external code from modifying the operation's outcome
//
// The caller can safely retrieve the result later using the ReadonlyAsyncResult.Result() method,
// which will block until the operation completes or the context is canceled.
func (a *AsyncNode[I, O]) RunType(ctx context.Context, input I) (*ReadonlyAsyncResult[O], error) {
	res := NewWritableAsyncResult[O](ctx)
	go func() {
		output, err := a.process(ctx, input)
		res.Set(output, err)
	}()
	return res.Fork(), nil
}

// Run implements the Node[I, any] interface by executing the processor asynchronously.
//
// Instead of returning the result directly, this method returns a ReadonlyAsyncResult
// which acts as a future/promise for the actual result. This allows the caller to
// continue execution without waiting for the processor to complete.
//
// The returned ReadonlyAsyncResult is wrapped as an 'any' type to conform to the Node
// interface. To access the strongly-typed AsyncResult, use RunType instead.
func (a *AsyncNode[I, O]) Run(ctx context.Context, input I) (any, error) {
	return a.RunType(ctx, input)
}

// WithProcessor assigns a processing function to this node.
//
// The processor function defines the operation that will be executed
// asynchronously when Run is called. It should take an input value
// and return a processed output value or an error.
//
// Returns the AsyncNode instance for method chaining.
//
// Example:
//
//	processor := &AsyncNode[Request, Response]{}
//	    .WithProcessor(func(ctx context.Context, req Request) (Response, error) {
//	        // Process the request
//	        return Response{Data: processed}, nil
//	    })
//
//	result, _ := processor.Run(ctx, request)
//	// Continue execution while processing happens in background
func (a *AsyncNode[I, O]) WithProcessor(processor Processor[I, O]) *AsyncNode[I, O] {
	a.withProcessor(processor)
	return a
}
