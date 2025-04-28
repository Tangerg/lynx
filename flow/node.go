// Package flow provides a robust, composable pipeline framework for creating data processing workflows.
//
// The flow package enables developers to build complex data processing pipelines through a set
// of composable nodes that can be connected in various configurations. The framework supports:
//
//   - Sequential processing with SequenceNode
//   - Conditional branching with BranchNode
//   - Iterative processing with LoopNode
//   - Batch processing with BatchNode
//   - Concurrent execution with ParallelNode
//   - Asynchronous processing with AsyncNode
//
// Each node type implements the Node interface, providing a standardized way to run processing
// logic and connect nodes together. The framework is designed for flexibility, allowing developers
// to express complex workflows while maintaining clear semantics and separation of concerns.
//
// Basic Usage:
//
//	// Create a simple processing pipeline
//	processor := &flow.SequenceNode{}
//	processor.WithProcessor(func(ctx context.Context, input any) (any, error) {
//		// Transform the input
//		return input.(string) + " processed", nil
//	})
//
// More complex workflows can be built by chaining and nesting different node types.
package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// processInput executes the node's processor function with appropriate context handling.
//
// This method performs validation, checks context status, and runs the processing function.
// It's a central component used by all node implementations to execute their logic.
//
// Returns an error if:
//   - No processor function is defined
//   - The context has been canceled
//   - The processor function itself returns an error
func (n *core[I, O]) processInput(ctx context.Context, input I) (O, error) {
	var res O
	if n.processor == nil {
		return res, errors.New("processor function is required")
	}
	err := n.ensureContextActive(ctx)
	if err != nil {
		return res, err
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

// SequenceNode implements a linear, sequential processing node in a workflow.
//
// SequenceNode represents the most basic form of pipeline processing, where
// data flows through a series of operations in a defined order. The output of
// each node becomes the input to the next node in the sequence.
//
// SequenceNodes are ideal for straightforward transformations where each step
// depends on the results of the previous step.
//
// Example:
//
//	// Create a pipeline that formats and then encrypts data
//	formatter := &flow.SequenceNode{}.WithProcessor(formatData)
//	encryptor := &flow.SequenceNode{}.WithProcessor(encryptData)
//	pipeline := formatter.WithSuccessor(encryptor)
//
//	// Process input through the pipeline
//	result, err := pipeline.Run(ctx, rawData)
type SequenceNode struct {
	core[any, any]
	// successor is the next node in the processing chain
	successor Node[any, any]
}

// Run processes the input and passes the result to the successor node.
//
// The method first applies this node's processing function to the input.
// If successful and a successor exists, it then passes the output to
// the successor node. This creates a chain of processing steps.
//
// If no successor is defined, returns the output of this node directly.
func (s *SequenceNode) Run(ctx context.Context, input any) (any, error) {
	output, err := s.processInput(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.successor == nil {
		return output, nil
	}
	return s.successor.Run(ctx, output)
}

// WithProcessor assigns a processing function to this node.
//
// The processor function defines the transformation logic for this node
// and will be executed when Run is called. It should take an input value
// and return a processed output value or an error.
//
// Returns the SequenceNode instance for method chaining.
func (s *SequenceNode) WithProcessor(processor Processor[any, any]) *SequenceNode {
	s.withProcessor(processor)
	return s
}

// WithSuccessor sets the next node in the processing chain.
//
// This method creates a sequential flow where the output of this node
// becomes the input to the specified successor node. Multiple nodes
// can be chained together to form a processing pipeline.
//
// Returns the SequenceNode instance for method chaining.
func (s *SequenceNode) WithSuccessor(successor Node[any, any]) *SequenceNode {
	if successor != nil {
		s.successor = successor
	}
	return s
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
	// routeSelector determines which branch to follow based on input and output
	routeSelector func(context.Context, any, any) (string, error)
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
	if b.routeSelector == nil {
		return nil, nil
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return nil, err
	}
	route, err := b.routeSelector(ctx, input, output)
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
	output, err := b.processInput(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(b.branches) == 0 {
		return output, nil
	}
	branch, err := b.resolveRoute(ctx, input, output)
	if err != nil {
		return nil, err
	}
	return branch.Run(ctx, output)
}

// WithRouteSelector assigns a function that determines the branch to follow.
//
// The route selector examines the input and output of this node's processor
// and returns a string identifier for the branch to follow. This string must
// match one of the branch keys added with AddBranch.
//
// Returns the BranchNode instance for method chaining.
func (b *BranchNode) WithRouteSelector(routeSelector func(context.Context, any, any) (string, error)) *BranchNode {
	if routeSelector != nil {
		b.routeSelector = routeSelector
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
	// stopCondition determines when to stop looping based on iteration count, input, and output
	stopCondition func(context.Context, int, I, O) (bool, error)
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
	if l.stopCondition == nil {
		return true, nil
	}
	err := l.ensureContextActive(ctx)
	if err != nil {
		return false, err
	}
	return l.stopCondition(ctx, iteration, input, output)
}

// executeIteration performs recursive loop execution with iteration tracking.
//
// This internal method handles the actual looping mechanism, maintaining
// the iteration count and evaluating the termination condition after each run.
//
// If the stop condition returns true or an error occurs, the loop terminates.
// Otherwise, it recursively calls itself with an incremented iteration count.
func (l *LoopNode[I, O]) executeIteration(ctx context.Context, iteration int, input I) (O, error) {
	var res O
	output, err := l.processInput(ctx, input)
	if err != nil {
		return res, err
	}
	shouldTerminate, err := l.shouldTerminate(ctx, iteration, input, output)
	if err != nil {
		return res, err
	}
	if shouldTerminate {
		return output, nil
	}
	return l.executeIteration(ctx, iteration+1, input)
}

// Run executes the processing logic in a loop until termination.
//
// This method initiates the looping process by calling executeIteration
// with an initial iteration count of 0. The final output (after all iterations)
// is returned when the loop terminates.
func (l *LoopNode[I, O]) Run(ctx context.Context, input I) (O, error) {
	return l.executeIteration(ctx, 0, input)
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

// WithStopCondition assigns a function that determines when to stop looping.
//
// The condition function examines:
//   - The current iteration count
//   - The original input to the loop
//   - The output from the most recent iteration
//
// It should return true when looping should stop, or false to continue.
//
// Returns the LoopNode instance for method chaining.
func (l *LoopNode[I, O]) WithStopCondition(condition func(context.Context, int, I, O) (bool, error)) *LoopNode[I, O] {
	if condition != nil {
		l.stopCondition = condition
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
type BatchNode[I any, O any, SI any, SO any] struct {
	core[SI, SO]
	// segmenter divides the input into multiple segments for processing
	segmenter func(context.Context, I) ([]SI, error)
	// aggregator combines processed segments back into a single result
	aggregator func(context.Context, []SO) (O, error)
}

// createSegments divides the input into multiple chunks for processing.
//
// This method determines how to split the input:
//   - If input is already a slice, it uses that directly
//   - If a segmenter function is defined, it calls that to divide the input
//   - Otherwise, it creates a single-item slice containing the original input
//
// Returns a slice of segments to be processed individually.
func (b *BatchNode[I, O, SI, SO]) createSegments(ctx context.Context, input I) ([]SI, error) {
	var res []SI
	if b.segmenter == nil {
		return res, nil
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return nil, err
	}
	return b.segmenter(ctx, input)
}

// aggregateResults combines the processed outputs into a single result.
//
// This method merges the individual segment results:
//   - If an aggregator function is defined, it calls that to combine results
//   - Otherwise, it returns the slice of results directly
//
// Returns the combined or aggregated result.
func (b *BatchNode[I, O, SI, SO]) aggregateResults(ctx context.Context, results []SO) (O, error) {
	var res O
	if b.aggregator == nil {
		return res, nil
	}
	err := b.ensureContextActive(ctx)
	if err != nil {
		return res, err
	}
	return b.aggregator(ctx, results)
}

// Run processes each segment individually and combines the results.
//
// The method:
//  1. Divides the input into segments
//  2. Processes each segment with the processor function
//  3. Collects all successful results
//  4. Aggregates the results into a final output
//
// If all segments fail, returns a joined error. If some succeed,
// returns the aggregated results of successful operations.
func (b *BatchNode[I, O, SI, SO]) Run(ctx context.Context, input I) (O, error) {
	var res O
	segments, err := b.createSegments(ctx, input)
	if err != nil {
		return res, err
	}
	var (
		results = make([]SO, 0, len(segments))
		errs    = make([]error, 0, len(segments))
	)

	for _, segment := range segments {
		output, err1 := b.processInput(ctx, segment)
		if err1 != nil {
			errs = append(errs, err1)
			continue
		}
		results = append(results, output)
	}
	if len(results) == 0 {
		return res, errors.Join(errs...)
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
func (b *BatchNode[I, O, SI, SO]) WithSegmenter(segmenter func(context.Context, I) ([]SI, error)) *BatchNode[I, O, SI, SO] {
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
func (b *BatchNode[I, O, SI, SO]) WithAggregator(aggregator func(context.Context, []SO) (O, error)) *BatchNode[I, O, SI, SO] {
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
func (b *BatchNode[I, O, SI, SO]) WithProcessor(processor Processor[SI, SO]) *BatchNode[I, O, SI, SO] {
	b.withProcessor(processor)
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
// The number of concurrent goroutines is determined by the number of segments,
// so be cautious with very large collections to avoid resource exhaustion.
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
type ParallelNode[I any, O any, SI any, SO any] struct {
	BatchNode[I, O, SI, SO]
}

// Run processes each segment concurrently and combines the results.
//
// This method overrides BatchNode.Run to use goroutines for concurrent execution:
//  1. Divides the input into segments
//  2. Launches a goroutine for each segment
//  3. Collects results as they complete
//  4. Waits for all processing to finish
//  5. Aggregates successful results
//
// Uses sync.WaitGroup to coordinate goroutines and sync.Mutex to safely
// collect results from concurrent operations.
func (p *ParallelNode[I, O, SI, SO]) Run(ctx context.Context, input I) (O, error) {
	var res O
	segments, err := p.createSegments(ctx, input)
	if err != nil {
		return res, err
	}
	var (
		results = make([]SO, 0, len(segments))
		errs    = make([]error, 0, len(segments))
		wg      sync.WaitGroup
		mu      sync.Mutex
	)
	for _, segment := range segments {
		wg.Add(1)
		go func(segment SI) {
			defer wg.Done()

			output, err1 := p.processInput(ctx, segment)
			mu.Lock()
			if err1 != nil {
				errs = append(errs, err1)
			} else {
				results = append(results, output)
			}
			mu.Unlock()
		}(segment)
	}
	wg.Wait()

	if len(results) == 0 {
		return res, errors.Join(errs...)
	}
	return p.aggregateResults(ctx, results)
}

// WithProcessor assigns a processing function to this node.
//
// The processor function will be applied to each segment concurrently.
// It should be thread-safe as it will be executed in separate goroutines.
//
// Returns the ParallelNode instance for method chaining.
func (p *ParallelNode[I, O, SI, SO]) WithProcessor(processor Processor[SI, SO]) *ParallelNode[I, O, SI, SO] {
	p.withProcessor(processor)
	return p
}
