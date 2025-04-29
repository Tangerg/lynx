package flow

import (
	"context"
	"errors"
)

// Flow represents a complete data processing pipeline and implements the Node interface itself.
//
// Flow is the fundamental sequential node chain, consisting of a series of processing nodes
// that execute in order. Since Flow itself implements the Node interface, it can be embedded
// within other nodes or combined with other nodes to form more complex processing structures.
//
// This self-recursive design allows Flow to be used both as a simple linear processing chain
// and as a building block in more sophisticated compositions. Through its fluent API, you can
// construct complex processing topologies by combining Flows with branches, loops, parallel
// execution, and other specialized node types.
type Flow struct {
	// nodes represents the sequence of processing steps in this flow
	nodes []Node[any, any]
}

// NewFlow creates a new, empty Flow ready for configuration.
//
// This is the entry point for building a data processing pipeline.
// The returned Flow can be configured with various node types through
// the builder methods (Step, Branch, Loop, Batch, Parallel).
func NewFlow() *Flow {
	return &Flow{
		nodes: make([]Node[any, any], 0),
	}
}

// validate checks if the Flow has been properly configured.
//
// Returns an error if the Flow has no processing nodes,
// as a valid Flow must contain at least one node.
func (f *Flow) validate() error {
	if len(f.nodes) == 0 {
		return errors.New("flow must contain at least one node")
	}
	return nil
}

// Run executes the Flow's processing pipeline with the provided input.
//
// This method:
// 1. Processes the input through the first node
// 2. Passes the output to the next node in sequence
// 3. Returns the final result after all nodes complete processing
//
// If no nodes are configured for this Flow, returns an error.
func (f *Flow) Run(ctx context.Context, input any) (any, error) {
	err := f.validate()
	if err != nil {
		return nil, err
	}
	var output = input
	for _, node := range f.nodes {
		output, err = node.Run(ctx, output)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}

// Then adds a pre-configured Node to this Flow.
//
// This method allows for incorporating an existing Node implementation
// into the pipeline. The node is appended to the sequence of processing
// steps in this Flow.
//
// This provides an escape hatch for cases where the builder methods
// don't offer the desired flexibility.
func (f *Flow) Then(node Node[any, any]) *Flow {
	if node != nil {
		f.nodes = append(f.nodes, node)
	}
	return f
}

// Step starts building a StepNode in this Flow.
//
// Returns a builder for configuring a simple processing node
// that transforms input data into output data in a single step.
//
// Example:
//
//	flow.NewFlow().Step().
//	    WithProcessor(processData).
//	    Then()
func (f *Flow) Step() *StepNodeBuilder {
	return newStepNodeBuilder(f)
}

// Branch starts building a BranchNode in this Flow.
//
// Returns a builder for configuring a conditional branching node,
// which routes data to different processing paths based on conditions.
//
// Example:
//
//	flow.NewFlow().Branch().
//	    WithProcessor(decodeInput).
//	    WithRouteResolver(determineRoute).
//	    AddBranch("route1", handler1).
//	    AddBranch("route2", handler2).
//	    Then()
func (f *Flow) Branch() *BranchNodeBuilder {
	return newBranchNodeBuilder(f)
}

// Loop starts building a LoopNode in this Flow.
//
// Returns a builder for configuring an iterative processing node,
// which repeatedly processes data until a termination condition is met.
//
// Example:
//
//	flow.NewFlow().Loop().
//	    WithProcessor(processIteration).
//	    WithTerminator(func(ctx context.Context, i int, in, out any) (bool, error) {
//	        return i >= 5, nil  // Stop after 5 iterations
//	    }).
//	    Then()
func (f *Flow) Loop() *LoopNodeBuilder {
	return newLoopNodeBuilder(f)
}

// Batch starts building a BatchNode in this Flow.
//
// Returns a builder for configuring a batch processing node,
// which divides data into segments, processes each segment,
// and recombines the results.
//
// Example:
//
//	flow.NewFlow().Batch().
//	    WithSegmenter(splitIntoChunks).
//	    WithProcessor(processChunk).
//	    WithAggregator(combineResults).
//	    Then()
func (f *Flow) Batch() *BatchNodeBuilder {
	return newBatchNodeBuilder(f)
}

// Parallel starts building a ParallelNode in this Flow.
//
// Returns a builder for configuring a concurrent processing node,
// which processes data segments in parallel for improved performance.
//
// Example:
//
//	flow.NewFlow().Parallel().
//	    WithSegmenter(splitIntoIndependentTasks).
//	    WithProcessor(processTask).
//	    WithAggregator(combineTaskResults).
//	    Then()
func (f *Flow) Parallel() *ParallelNodeBuilder {
	return newParallelNodeBuilder(f)
}

// Async creates a new AsyncNodeBuilder to add asynchronous processing capabilities to the flow.
//
// AsyncNode allows the flow to execute a processor asynchronously, returning immediately with
// an AsyncResult that can be used to retrieve the result later. This is useful for long-running
// operations where you want to continue execution without waiting for completion.
//
// Example:
//
//	flow.NewFlow().
//	  Step().WithProcessor(preProcess).
//	  Then().
//	  Async().WithProcessor(longRunningProcess).
//	  Then()
//
// The result from an async node is an AsyncResult which can be used to retrieve the actual
// value when needed:
//
//	asyncResult, _ := flow.Run(ctx, input)
//	result, err := asyncResult.Get() // Blocks until result is available
func (f *Flow) Async() *AsyncNodeBuilder {
	return newAsyncNodeBuilder(f)
}

// StepNodeBuilder provides a fluent API for configuring a StepNode.
//
// This builder simplifies the process of creating and configuring simple
// processing nodes within a Flow pipeline.
type StepNodeBuilder struct {
	flow *Flow
	// node is the StepNode being configured by this builder
	node *StepNode[any, any]
}

// newStepNodeBuilder creates a new builder for configuring a StepNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh StepNode to be configured.
func newStepNodeBuilder(flow *Flow) *StepNodeBuilder {
	return &StepNodeBuilder{
		flow: flow,
		node: &StepNode[any, any]{},
	}
}

// WithProcessor assigns a processing function to the StepNode.
//
// The processor function defines the transformation that will be applied
// when the node is executed.
//
// Returns the builder for method chaining.
func (s *StepNodeBuilder) WithProcessor(processor Processor[any, any]) *StepNodeBuilder {
	s.node.WithProcessor(processor)
	return s
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// StepNode into the flow pipeline and returning the flow for further
// configuration.
func (s *StepNodeBuilder) Then() *Flow {
	return s.flow.Then(s.node)
}

// BranchNodeBuilder provides a fluent API for configuring a BranchNode.
//
// This builder simplifies the process of creating and configuring conditional
// branching nodes within a Flow pipeline.
type BranchNodeBuilder struct {
	flow *Flow
	// node is the BranchNode being configured by this builder
	node *BranchNode
}

// newBranchNodeBuilder creates a new builder for configuring a BranchNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh BranchNode to be configured.
func newBranchNodeBuilder(flow *Flow) *BranchNodeBuilder {
	return &BranchNodeBuilder{
		flow: flow,
		node: &BranchNode{},
	}
}

// WithRouteResolver assigns a function that determines the branch to follow.
//
// The route resolver examines the input and output of this node's processor
// and returns a string identifier for the branch to follow.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) WithRouteResolver(routeResolver func(context.Context, any, any) (string, error)) *BranchNodeBuilder {
	b.node.WithRouteResolver(routeResolver)
	return b
}

// WithProcessor assigns a processing function to the BranchNode.
//
// The processor function transforms the input before branching decisions are made.
// The output of this processor will be passed to the route resolver.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) WithProcessor(processor Processor[any, any]) *BranchNodeBuilder {
	b.node.WithProcessor(processor)
	return b
}

// AddBranch adds a new branch node with the given identifier.
//
// The route string is used as the key for selecting this branch when the
// route resolver returns a matching identifier.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) AddBranch(route string, node Node[any, any]) *BranchNodeBuilder {
	b.node.AddBranch(route, node)
	return b
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// BranchNode into the flow pipeline and returning the flow
// for further configuration.
func (b *BranchNodeBuilder) Then() *Flow {
	return b.flow.Then(b.node)
}

// LoopNodeBuilder provides a fluent API for configuring a LoopNode.
//
// This builder simplifies the process of creating and configuring iterative
// processing nodes within a Flow pipeline.
type LoopNodeBuilder struct {
	flow *Flow
	// node is the LoopNode being configured by this builder
	node *LoopNode[any, any]
}

// newLoopNodeBuilder creates a new builder for configuring a LoopNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh LoopNode to be configured.
func newLoopNodeBuilder(flow *Flow) *LoopNodeBuilder {
	return &LoopNodeBuilder{
		flow: flow,
		node: &LoopNode[any, any]{},
	}
}

// WithProcessor assigns a processing function to the LoopNode.
//
// The processor function will be executed repeatedly in each iteration of the loop.
// Its output from one iteration becomes its input for the next iteration.
//
// Returns the builder for method chaining.
func (l *LoopNodeBuilder) WithProcessor(processor Processor[any, any]) *LoopNodeBuilder {
	l.node.WithProcessor(processor)
	return l
}

// WithTerminator assigns a function that determines when to stop looping.
//
// The terminator function examines the current iteration count, the original input,
// and the output from the most recent iteration to decide when to terminate.
//
// Returns the builder for method chaining.
func (l *LoopNodeBuilder) WithTerminator(terminator func(context.Context, int, any, any) (bool, error)) *LoopNodeBuilder {
	l.node.WithTerminator(terminator)
	return l
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// LoopNode into the flow pipeline and returning the flow
// for further configuration.
func (l *LoopNodeBuilder) Then() *Flow {
	return l.flow.Then(l.node)
}

// BatchNodeBuilder provides a fluent API for configuring a BatchNode.
//
// This builder simplifies the process of creating and configuring batch
// processing nodes within a Flow pipeline.
type BatchNodeBuilder struct {
	flow *Flow
	// node is the BatchNode being configured by this builder
	node *BatchNode[any, any, any, any]
}

// newBatchNodeBuilder creates a new builder for configuring a BatchNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh BatchNode to be configured.
func newBatchNodeBuilder(flow *Flow) *BatchNodeBuilder {
	return &BatchNodeBuilder{
		flow: flow,
		node: &BatchNode[any, any, any, any]{},
	}
}

// WithSegmenter assigns a function that divides input into multiple chunks.
//
// The segmenter function takes a single input and returns a slice of items
// to be processed individually.
//
// Returns the builder for method chaining.
func (b *BatchNodeBuilder) WithSegmenter(segmenter func(context.Context, any) ([]any, error)) *BatchNodeBuilder {
	b.node.WithSegmenter(segmenter)
	return b
}

// WithAggregator assigns a function that combines processed results.
//
// The aggregator function takes a slice of individually processed results
// and combines them into a single output.
//
// Returns the builder for method chaining.
func (b *BatchNodeBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *BatchNodeBuilder {
	b.node.WithAggregator(aggregator)
	return b
}

// WithProcessor assigns a processing function to the BatchNode.
//
// The processor function will be applied to each individual segment
// during batch processing.
//
// Returns the builder for method chaining.
func (b *BatchNodeBuilder) WithProcessor(processor Processor[any, any]) *BatchNodeBuilder {
	b.node.WithProcessor(processor)
	return b
}

// AllowFailure configures whether the batch processing should continue
// when individual segment processing fails.
//
// When set to true, processing continues even if some segments fail.
// When false (default), processing stops on the first error.
//
// Returns the builder for method chaining.
func (b *BatchNodeBuilder) AllowFailure(allowFailure bool) *BatchNodeBuilder {
	b.node.AllowFailure(allowFailure)
	return b
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// BatchNode into the flow pipeline and returning the flow
// for further configuration.
func (b *BatchNodeBuilder) Then() *Flow {
	return b.flow.Then(b.node)
}

// ParallelNodeBuilder provides a fluent API for configuring a ParallelNode.
//
// This builder simplifies the process of creating and configuring concurrent
// processing nodes within a Flow pipeline.
type ParallelNodeBuilder struct {
	flow *Flow
	// node is the ParallelNode being configured by this builder
	node *ParallelNode[any, any, any, any]
}

// newParallelNodeBuilder creates a new builder for configuring a ParallelNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh ParallelNode to be configured.
func newParallelNodeBuilder(flow *Flow) *ParallelNodeBuilder {
	return &ParallelNodeBuilder{
		flow: flow,
		node: &ParallelNode[any, any, any, any]{},
	}
}

// WithSegmenter assigns a function that divides input into multiple chunks.
//
// The segmenter function takes a single input and returns a slice of items
// to be processed concurrently.
//
// Returns the builder for method chaining.
func (p *ParallelNodeBuilder) WithSegmenter(segmenter func(context.Context, any) ([]any, error)) *ParallelNodeBuilder {
	p.node.WithSegmenter(segmenter)
	return p
}

// WithAggregator assigns a function that combines processed results.
//
// The aggregator function takes a slice of concurrently processed results
// and combines them into a single output.
//
// Returns the builder for method chaining.
func (p *ParallelNodeBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *ParallelNodeBuilder {
	p.node.WithAggregator(aggregator)
	return p
}

// WithProcessor assigns a processing function to the ParallelNode.
//
// The processor function will be applied to each segment concurrently.
// It should be thread-safe as it will be executed in separate goroutines.
//
// Returns the builder for method chaining.
func (p *ParallelNodeBuilder) WithProcessor(processor Processor[any, any]) *ParallelNodeBuilder {
	p.node.WithProcessor(processor)
	return p
}

// AllowFailure configures whether the parallel processing should continue
// when individual segment processing fails.
//
// When set to true, processing continues even if some segments fail.
// When false (default), processing stops on the first error.
//
// Returns the builder for method chaining.
func (p *ParallelNodeBuilder) AllowFailure(allowFailure bool) *ParallelNodeBuilder {
	p.node.AllowFailure(allowFailure)
	return p
}

// Limit sets the maximum number of concurrent goroutines to use for processing.
//
// This allows control over resource utilization when processing large batches.
// A value <= 0 means no limit (use a goroutine for each segment).
//
// Returns the builder for method chaining.
func (p *ParallelNodeBuilder) Limit(limit int) *ParallelNodeBuilder {
	p.node.Limit(limit)
	return p
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// ParallelNode into the flow pipeline and returning the flow
// for further configuration.
func (p *ParallelNodeBuilder) Then() *Flow {
	return p.flow.Then(p.node)
}

// AsyncNodeBuilder provides a fluent API for configuring an AsyncNode before adding it to a Flow.
//
// AsyncNode executes its processor in a separate goroutine and immediately returns an
// AsyncResult that will eventually contain the result of the processing.
type AsyncNodeBuilder struct {
	flow *Flow
	node *AsyncNode[any, any]
}

// newAsyncNodeBuilder creates a new AsyncNodeBuilder for the given flow.
//
// The builder is used to configure the AsyncNode before adding it to the flow.
func newAsyncNodeBuilder(flow *Flow) *AsyncNodeBuilder {
	return &AsyncNodeBuilder{
		flow: flow,
		node: &AsyncNode[any, any]{},
	}
}

// WithProcessor sets the processing function that will be executed asynchronously.
//
// The processor receives input data and returns output data or an error. It will be
// executed in a separate goroutine, and its result will be available through an AsyncResult.
//
// Returns the builder for method chaining.
func (a *AsyncNodeBuilder) WithProcessor(processor Processor[any, any]) *AsyncNodeBuilder {
	a.node.WithProcessor(processor)
	return a
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// ParallelNode into the flow pipeline and returning the flow
// for further configuration.
func (a *AsyncNodeBuilder) Then() *Flow {
	return a.flow.Then(a.node)
}
