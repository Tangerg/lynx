// Package flow Flow represents a complete data processing pipeline composed of connected nodes.
//
// Flow provides a fluent API for building complex processing workflows by chaining
// different node types together. It serves as the primary entry point for constructing
// and executing data processing pipelines.
//
// The Flow structure manages the connections between nodes and handles passing data
// from one node to the next in the pipeline, making it easy to build sophisticated
// data transformation workflows.
package flow

import (
	"context"
	"errors"
)

// Flow represents a complete data processing pipeline.
//
// A Flow consists of a processing node and an optional successor Flow.
// This structure enables the creation of complex, chainable processing
// pipelines through a fluent API.
type Flow struct {
	// node represents the current processing step in this flow
	node Node[any, any]
	// successor is the next Flow in the processing chain
	successor *Flow
}

// NewFlow creates a new, empty Flow ready for configuration.
//
// This is the entry point for building a data processing pipeline.
// The returned Flow can be configured with various node types through
// the builder methods (Sequence, Branch, Loop, etc.).
func NewFlow() *Flow {
	return &Flow{}
}

// Run executes the Flow's processing pipeline with the provided input.
//
// This method:
// 1. Processes the input through the current node
// 2. Passes the output to the successor Flow, if one exists
// 3. Returns the final result after all connected flows complete
//
// If no node is configured for this Flow, returns nil, nil to allow
// for placeholder flows in a pipeline.
func (f *Flow) Run(ctx context.Context, input any) (any, error) {
	if f.node == nil {
		return nil, errors.New("flow node is required")
	}
	output, err := f.node.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	if f.successor == nil {
		return output, nil
	}
	return f.successor.Run(ctx, output)
}

// Then creates a successor Flow and returns it for further configuration.
//
// This method enables the fluent chaining of processing steps in a pipeline,
// allowing for the construction of multi-step workflows.
//
// If no node is set in the current Flow, returns the current Flow instead
// of creating a successor to avoid empty placeholder flows.
func (f *Flow) Then() *Flow {
	if f.node == nil {
		return f
	}
	f.successor = NewFlow()
	return f.successor
}

// Compile validates and prepares the Flow for execution.
//
// This method performs validation to ensure the Flow is properly configured:
// - Checks that the current node is defined
// - Prunes any trailing empty successor flows
//
// It returns the Flow itself as a Node[any, any], allowing the configured
// Flow to be used directly in contexts that expect a Node interface.
// This enables reuse of flow configurations and composition of Flows
// within larger processing pipelines.
//
// Returns an error if the Flow is not properly configured (e.g., no node defined).
//
// Example:
//
//	pipeline, err := flow.NewFlow().
//	    Sequence().
//	    WithProcessor(firstStep).
//	    Then().
//	    Parallel().
//	    WithProcessor(secondStep).
//	    Then().
//	    Compile()
//
//	if err != nil {
//	    // Handle configuration error
//	}
//
//	// Use the compiled pipeline
//	result, err := pipeline.Run(ctx, input)
func (f *Flow) Compile() (Node[any, any], error) {
	if f.node == nil {
		return nil, errors.New("flow node is required")
	}
	if f.successor != nil && f.successor.node == nil {
		f.successor = nil
	}
	return f, nil
}

// WithNode adds a pre-configured Node to this Flow.
//
// This method allows for incorporating an existing Node implementation
// into the pipeline. After setting the node, it creates and returns a
// successor Flow for further pipeline configuration.
//
// This provides an escape hatch for cases where the builder methods
// don't offer the desired flexibility.
func (f *Flow) WithNode(node Node[any, any]) *Flow {
	f.node = node
	return f.Then()
}

// Sequence starts building a SequenceNode in this Flow.
//
// Returns a builder for configuring a sequential processing node,
// which performs a single transformation on the data.
//
// Example:
//
//	flow.NewFlow().Sequence().
//	    WithProcessor(func(ctx context.Context, input any) (any, error) {
//	        return fmt.Sprintf("Processed: %v", input), nil
//	    }).
//	    Then()
func (f *Flow) Sequence() *SequenceNodeBuilder {
	return newSequenceNodeBuilder(f)
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
//	    WithRouteSelector(determineRoute).
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
//	    WithStopCondition(func(ctx context.Context, i int, in, out any) (bool, error) {
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

// builder provides common functionality for all node builders.
//
// This internal type serves as a base for the specialized node builders,
// handling the common pattern of configuring a node and integrating it
// into the flow pipeline.
type builder struct {
	// flow is the Flow that this builder is configuring
	flow *Flow
}

// build finalizes the node configuration and adds it to the flow.
//
// This internal method completes the builder process by:
// 1. Setting the configured node as the current node in the flow
// 2. Creating a successor flow for further pipeline configuration
// 3. Returning the successor for method chaining
//
// This shared implementation reduces code duplication across builders.
func (b *builder) build(node Node[any, any]) *Flow {
	b.flow.node = node
	return b.flow.Then()
}

// SequenceNodeBuilder provides a fluent API for configuring a SequenceNode.
//
// This builder simplifies the process of creating and configuring sequential
// processing nodes within a Flow pipeline.
type SequenceNodeBuilder struct {
	builder
	// node is the SequenceNode being configured by this builder
	node *SequenceNode
}

// newSequenceNodeBuilder creates a new builder for configuring a SequenceNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh SequenceNode to be configured.
func newSequenceNodeBuilder(flow *Flow) *SequenceNodeBuilder {
	return &SequenceNodeBuilder{
		builder: builder{flow: flow},
		node:    &SequenceNode{},
	}
}

// WithProcessor assigns a processing function to the SequenceNode.
//
// The processor function defines the transformation logic for this node
// and will be executed when the pipeline runs.
//
// Returns the builder for method chaining.
func (s *SequenceNodeBuilder) WithProcessor(processor Processor[any, any]) *SequenceNodeBuilder {
	s.node.withProcessor(processor)
	return s
}

// WithSuccessor sets the next node in the processing chain.
//
// This method creates a sequential flow where the output of this node
// becomes the input to the specified successor node.
//
// Returns the builder for method chaining.
func (s *SequenceNodeBuilder) WithSuccessor(successor Node[any, any]) *SequenceNodeBuilder {
	s.node.WithSuccessor(successor)
	return s
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// SequenceNode into the flow pipeline and returning the successor flow
// for further configuration.
func (s *SequenceNodeBuilder) Then() *Flow {
	return s.build(s.node)
}

// BranchNodeBuilder provides a fluent API for configuring a BranchNode.
//
// This builder simplifies the process of creating and configuring conditional
// branching nodes within a Flow pipeline.
type BranchNodeBuilder struct {
	builder
	// node is the BranchNode being configured by this builder
	node *BranchNode
}

// newBranchNodeBuilder creates a new builder for configuring a BranchNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh BranchNode to be configured.
func newBranchNodeBuilder(flow *Flow) *BranchNodeBuilder {
	return &BranchNodeBuilder{
		builder: builder{flow: flow},
		node:    &BranchNode{},
	}
}

// WithRouteSelector assigns a function that determines the branch to follow.
//
// The route selector examines the input and output of this node's processor
// and returns a string identifier for the branch to follow.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) WithRouteSelector(routeSelector func(context.Context, any, any) (string, error)) *BranchNodeBuilder {
	b.node.WithRouteSelector(routeSelector)
	return b
}

// WithProcessor assigns a processing function to the BranchNode.
//
// The processor function transforms the input before branching decisions are made.
// The output of this processor will be passed to the route selector.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) WithProcessor(processor Processor[any, any]) *BranchNodeBuilder {
	b.node.WithProcessor(processor)
	return b
}

// AddBranch adds a new branch node with the given identifier.
//
// The route string is used as the key for selecting this branch when the
// route selector returns a matching identifier.
//
// Returns the builder for method chaining.
func (b *BranchNodeBuilder) AddBranch(route string, node Node[any, any]) *BranchNodeBuilder {
	b.node.AddBranch(route, node)
	return b
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// BranchNode into the flow pipeline and returning the successor flow
// for further configuration.
func (b *BranchNodeBuilder) Then() *Flow {
	return b.build(b.node)
}

// LoopNodeBuilder provides a fluent API for configuring a LoopNode.
//
// This builder simplifies the process of creating and configuring iterative
// processing nodes within a Flow pipeline.
type LoopNodeBuilder struct {
	builder
	// node is the LoopNode being configured by this builder
	node *LoopNode[any, any]
}

// newLoopNodeBuilder creates a new builder for configuring a LoopNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh LoopNode to be configured.
func newLoopNodeBuilder(flow *Flow) *LoopNodeBuilder {
	return &LoopNodeBuilder{
		builder: builder{flow: flow},
		node:    &LoopNode[any, any]{},
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

// WithStopCondition assigns a function that determines when to stop looping.
//
// The condition function examines the current iteration count, the original input,
// and the output from the most recent iteration to decide when to terminate.
//
// Returns the builder for method chaining.
func (l *LoopNodeBuilder) WithStopCondition(condition func(context.Context, int, any, any) (bool, error)) *LoopNodeBuilder {
	l.node.WithStopCondition(condition)
	return l
}

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// LoopNode into the flow pipeline and returning the successor flow
// for further configuration.
func (l *LoopNodeBuilder) Then() *Flow {
	return l.build(l.node)
}

// BatchNodeBuilder provides a fluent API for configuring a BatchNode.
//
// This builder simplifies the process of creating and configuring batch
// processing nodes within a Flow pipeline.
type BatchNodeBuilder struct {
	builder
	// node is the BatchNode being configured by this builder
	node *BatchNode[any, any, any, any]
}

// newBatchNodeBuilder creates a new builder for configuring a BatchNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh BatchNode to be configured.
func newBatchNodeBuilder(flow *Flow) *BatchNodeBuilder {
	return &BatchNodeBuilder{
		builder: builder{flow: flow},
		node:    &BatchNode[any, any, any, any]{},
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

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// BatchNode into the flow pipeline and returning the successor flow
// for further configuration.
func (b *BatchNodeBuilder) Then() *Flow {
	return b.build(b.node)
}

// ParallelNodeBuilder provides a fluent API for configuring a ParallelNode.
//
// This builder simplifies the process of creating and configuring concurrent
// processing nodes within a Flow pipeline.
type ParallelNodeBuilder struct {
	builder
	// node is the ParallelNode being configured by this builder
	node *ParallelNode[any, any, any, any]
}

// newParallelNodeBuilder creates a new builder for configuring a ParallelNode.
//
// This internal function initializes the builder with a reference to the
// containing flow and creates a fresh ParallelNode to be configured.
func newParallelNodeBuilder(flow *Flow) *ParallelNodeBuilder {
	return &ParallelNodeBuilder{
		builder: builder{flow: flow},
		node:    &ParallelNode[any, any, any, any]{},
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

// Then completes the configuration of this node and adds it to the flow.
//
// This method finalizes the builder process, integrating the configured
// ParallelNode into the flow pipeline and returning the successor flow
// for further configuration.
func (p *ParallelNodeBuilder) Then() *Flow {
	return p.build(p.node)
}
