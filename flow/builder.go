package flow

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/Tangerg/lynx/pkg/sync"
)

// buildOnce provides atomic build-once semantics using lock-free operations.
// It ensures that a builder can only be built once, preventing configuration
// modification after the build phase.
type buildOnce struct {
	state atomic.Bool
}

// markBuilt atomically marks as built and returns true if this is the first call.
// Subsequent calls will return false, indicating the builder was already built.
func (b *buildOnce) markBuilt() bool {
	return b.state.CompareAndSwap(false, true)
}

// isBuilt checks if already built without modifying the state.
func (b *buildOnce) isBuilt() bool {
	return b.state.Load()
}

// ==================== LoopBuilder ====================

// LoopBuilder provides a fluent API for constructing Loop nodes.
// It accumulates configuration through method chaining.
// The builder is designed to be used within a closure passed to Builder.Loop().
type LoopBuilder struct {
	builder *Builder              // Reference to parent builder
	config  *LoopConfig[any, any] // Loop configuration being built
	once    buildOnce             // Ensures single build
}

// newLoopBuilder creates a new LoopBuilder instance.
// This is an internal constructor, users should use Builder.Loop() instead.
//
// Parameters:
//   - builder: Parent builder to which the loop will be added
func newLoopBuilder(builder *Builder) *LoopBuilder {
	return &LoopBuilder{
		builder: builder,
		config:  &LoopConfig[any, any]{},
	}
}

// WithNode sets the node to be executed in each iteration.
// The node receives the output of the previous iteration as input.
//
// Parameters:
//   - node: Node to execute in each loop iteration
//
// Returns the builder for method chaining.
func (l *LoopBuilder) WithNode(node Node[any, any]) *LoopBuilder {
	if node != nil {
		l.config.Node = node
	}
	return l
}

// WithMaxIterations sets the maximum number of iterations allowed.
// This provides a hard limit to prevent infinite loops.
//
// Parameters:
//   - maxIterations: Maximum iterations (must be > 0 to take effect, <= 0 is ignored)
//
// Returns the builder for method chaining.
func (l *LoopBuilder) WithMaxIterations(maxIterations int) *LoopBuilder {
	if maxIterations > 0 {
		l.config.MaxIterations = maxIterations
	}
	return l
}

// WithTerminator sets the function that determines when to stop looping.
// The terminator is called after each iteration to decide whether to continue.
//
// Parameters:
//   - terminator: Function that receives:
//   - ctx: Context for cancellation
//   - iteration: Current iteration number (0-based)
//   - input: Original input to the loop
//   - output: Output from the last iteration
//     Returns true to continue looping, false to stop.
//
// Returns the builder for method chaining.
func (l *LoopBuilder) WithTerminator(terminator func(context.Context, int, any, any) (bool, error)) *LoopBuilder {
	if terminator != nil {
		l.config.Terminator = terminator
	}
	return l
}

// build constructs the final Loop node from the accumulated configuration.
// This is called internally by Builder.Loop() after the configuration closure executes.
//
// Returns:
//   - The constructed Loop node
//   - Error if configuration is invalid or build was already called
func (l *LoopBuilder) build() (Node[any, any], error) {
	// Ensure build is called only once
	if !l.once.markBuilt() {
		return nil, errors.New("loop already built: build() can only be called once")
	}

	// Delegate to Loop constructor for validation and creation
	return NewLoop(l.config)
}

// ==================== BranchBuilder ====================

// BranchBuilder provides a fluent API for constructing Branch nodes.
// It accumulates branch configuration including the decision node,
// branch mappings, and resolution logic.
// The builder is designed to be used within a closure passed to Builder.Branch().
type BranchBuilder struct {
	builder *Builder      // Reference to parent builder
	config  *BranchConfig // Branch configuration being built
	once    buildOnce     // Ensures single build
}

// newBranchBuilder creates a new BranchBuilder instance.
// This is an internal constructor, users should use Builder.Branch() instead.
//
// Parameters:
//   - builder: Parent builder to which the branch will be added
func newBranchBuilder(builder *Builder) *BranchBuilder {
	return &BranchBuilder{
		builder: builder,
		config: &BranchConfig{
			Branches: make(map[string]Node[any, any]),
		},
	}
}

// WithNode sets the main decision node whose output determines which branch to take.
// The decision node is executed first, and its output is passed to the BranchResolver.
//
// Parameters:
//   - node: Decision node to execute
//
// Returns the builder for method chaining.
func (b *BranchBuilder) WithNode(node Node[any, any]) *BranchBuilder {
	if node != nil {
		b.config.Node = node
	}
	return b
}

// WithBranch adds a single branch mapping from name to node.
// Multiple branches can be added by calling this method multiple times.
//
// Parameters:
//   - branchName: Name of the branch (used by BranchResolver)
//   - node: Node to execute if this branch is selected
//
// Returns the builder for method chaining.
func (b *BranchBuilder) WithBranch(branchName string, node Node[any, any]) *BranchBuilder {
	if node != nil {
		b.config.Branches[branchName] = node
	}
	return b
}

// WithBranches sets all branches at once, replacing any previously configured branches.
// This is useful when branch configuration is computed dynamically.
//
// Parameters:
//   - branches: Map of branch names to nodes
//
// Returns the builder for method chaining.
func (b *BranchBuilder) WithBranches(branches map[string]Node[any, any]) *BranchBuilder {
	if branches != nil {
		b.config.Branches = branches
	}
	return b
}

// WithBranchResolver sets the function that determines which branch to execute.
// The resolver is called after the decision node executes.
//
// Parameters:
//   - resolver: Function that receives:
//   - ctx: Context for cancellation
//   - input: Original input to the decision node
//   - output: Output from the decision node
//     Returns the name of the branch to execute.
//
// Returns the builder for method chaining.
func (b *BranchBuilder) WithBranchResolver(resolver func(context.Context, any, any) (string, error)) *BranchBuilder {
	if resolver != nil {
		b.config.BranchResolver = resolver
	}
	return b
}

// build constructs the final Branch node from the accumulated configuration.
// This is called internally by Builder.Branch() after the configuration closure executes.
//
// Returns:
//   - The constructed Branch node
//   - Error if configuration is invalid or build was already called
func (b *BranchBuilder) build() (Node[any, any], error) {
	// Ensure build is called only once
	if !b.once.markBuilt() {
		return nil, errors.New("branch already built: build() can only be called once")
	}

	// Delegate to Branch constructor for validation and creation
	return NewBranch(b.config)
}

// ==================== BatchBuilder ====================

// BatchBuilder provides a fluent API for constructing Batch nodes.
// It accumulates batch processing configuration including segmentation,
// processing, aggregation, and concurrency control.
// The builder is designed to be used within a closure passed to Builder.Batch().
type BatchBuilder struct {
	builder *Builder                         // Reference to parent builder
	config  *BatchConfig[any, any, any, any] // Batch configuration being built
	once    buildOnce                        // Ensures single build
}

// newBatchBuilder creates a new BatchBuilder instance.
// This is an internal constructor, users should use Builder.Batch() instead.
//
// Parameters:
//   - builder: Parent builder to which the batch will be added
func newBatchBuilder(builder *Builder) *BatchBuilder {
	return &BatchBuilder{
		builder: builder,
		config:  &BatchConfig[any, any, any, any]{},
	}
}

// WithContinueOnError configures the batch to continue processing remaining segments
// even when one segment fails. By default, the batch stops on first error.
//
// Returns the builder for method chaining.
func (b *BatchBuilder) WithContinueOnError() *BatchBuilder {
	b.config.ContinueOnError = true
	return b
}

// WithConcurrencyLimit sets the maximum number of concurrent segment processors.
// This limits resource usage and prevents overwhelming downstream systems.
//
// Parameters:
//   - concurrencyLimit: Maximum concurrent workers (must be > 0 to take effect)
//
// Returns the builder for method chaining.
func (b *BatchBuilder) WithConcurrencyLimit(concurrencyLimit int) *BatchBuilder {
	if concurrencyLimit > 0 {
		b.config.ConcurrencyLimit = concurrencyLimit
	}
	return b
}

// WithNode sets the node to process each segment.
// This node is executed once per segment, potentially in parallel.
//
// Parameters:
//   - node: Node to execute for each segment
//
// Returns the builder for method chaining.
func (b *BatchBuilder) WithNode(node Node[any, any]) *BatchBuilder {
	if node != nil {
		b.config.Node = node
	}
	return b
}

// WithSegmenter sets the function that splits input into segments.
// Each segment will be processed independently by the node.
//
// Parameters:
//   - segmenter: Function that receives:
//   - ctx: Context for cancellation
//   - input: Original input to split
//     Returns a slice of segments to process.
//
// Returns the builder for method chaining.
func (b *BatchBuilder) WithSegmenter(segmenter func(context.Context, any) ([]any, error)) *BatchBuilder {
	if segmenter != nil {
		b.config.Segmenter = segmenter
	}
	return b
}

// WithAggregator sets the function that combines segment results into final output.
// This is called after all segments are processed (or after an error if ContinueOnError is false).
//
// Parameters:
//   - aggregator: Function that receives:
//   - ctx: Context for cancellation
//   - results: Slice of results from each segment (may contain nil for failed segments)
//     Returns the aggregated final output.
//
// Returns the builder for method chaining.
func (b *BatchBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *BatchBuilder {
	if aggregator != nil {
		b.config.Aggregator = aggregator
	}
	return b
}

// build constructs the final Batch node from the accumulated configuration.
// This is called internally by Builder.Batch() after the configuration closure executes.
//
// Returns:
//   - The constructed Batch node
//   - Error if configuration is invalid or build was already called
func (b *BatchBuilder) build() (Node[any, any], error) {
	// Ensure build is called only once
	if !b.once.markBuilt() {
		return nil, errors.New("batch already built: build() can only be called once")
	}

	// Delegate to Batch constructor for validation and creation
	return NewBatch(b.config)
}

// ==================== AsyncBuilder ====================

// AsyncBuilder provides a fluent API for constructing Async nodes.
// It accumulates async execution configuration including the node to execute
// and optional thread pool for execution.
// The builder is designed to be used within a closure passed to Builder.Async().
type AsyncBuilder struct {
	builder *Builder               // Reference to parent builder
	config  *AsyncConfig[any, any] // Async configuration being built
	once    buildOnce              // Ensures single build
}

// newAsyncBuilder creates a new AsyncBuilder instance.
// This is an internal constructor, users should use Builder.Async() instead.
//
// Parameters:
//   - builder: Parent builder to which the async node will be added
func newAsyncBuilder(builder *Builder) *AsyncBuilder {
	return &AsyncBuilder{
		builder: builder,
		config:  &AsyncConfig[any, any]{},
	}
}

// WithNode sets the node to be executed asynchronously.
// The node will run in a separate goroutine or thread pool worker.
//
// Parameters:
//   - node: Node to execute asynchronously
//
// Returns the builder for method chaining.
func (a *AsyncBuilder) WithNode(node Node[any, any]) *AsyncBuilder {
	if node != nil {
		a.config.Node = node
	}
	return a
}

// WithPool sets the thread pool for async execution.
// If not set, a default executor will be used that spawns a new goroutine.
//
// Parameters:
//   - pool: Thread pool for managing async execution
//
// Returns the builder for method chaining.
func (a *AsyncBuilder) WithPool(pool sync.Pool) *AsyncBuilder {
	if pool != nil {
		a.config.Pool = pool
	}
	return a
}

// build constructs the final Async node from the accumulated configuration.
// This is called internally by Builder.Async() after the configuration closure executes.
//
// Returns:
//   - The constructed Async node
//   - Error if configuration is invalid or build was already called
func (a *AsyncBuilder) build() (Node[any, any], error) {
	// Ensure build is called only once
	if !a.once.markBuilt() {
		return nil, errors.New("async already built: build() can only be called once")
	}

	// Delegate to Async constructor for validation and creation
	return NewAsync(a.config)
}

// ==================== ParallelBuilder ====================

// ParallelBuilder provides a fluent API for constructing Parallel nodes.
// It accumulates parallel execution configuration including nodes to execute,
// wait strategies, error handling, and result aggregation.
// The builder is designed to be used within a closure passed to Builder.Parallel().
type ParallelBuilder struct {
	builder *Builder                  // Reference to parent builder
	config  *ParallelConfig[any, any] // Parallel configuration being built
	once    buildOnce                 // Ensures single build
}

// newParallelBuilder creates a new ParallelBuilder instance.
// This is an internal constructor, users should use Builder.Parallel() instead.
//
// Parameters:
//   - builder: Parent builder to which the parallel node will be added
func newParallelBuilder(builder *Builder) *ParallelBuilder {
	return &ParallelBuilder{
		builder: builder,
		config:  &ParallelConfig[any, any]{},
	}
}

// WithWaitCount sets how many node completions to wait for before aggregating.
// This provides fine-grained control over parallel execution completion.
//
// Parameters:
//   - waitCount: Number of completions to wait for
//   - Positive number: Wait for exactly N completions
//   - -1: Wait for all nodes to complete (default behavior)
//   - 1: Wait for first completion (same as WithWaitAny)
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithWaitCount(waitCount int) *ParallelBuilder {
	p.config.WaitCount = waitCount
	return p
}

// WithWaitAny configures the parallel node to return as soon as the first node completes.
// This is a shorthand for WithWaitCount(1).
// Useful for scenarios like "fastest response wins" or "any valid result".
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithWaitAny() *ParallelBuilder {
	p.config.WaitCount = 1
	return p
}

// WithWaitAll configures the parallel node to wait for all nodes to complete.
// This is the default behavior and equivalent to WithWaitCount(-1).
// Useful for scenarios requiring all results for aggregation.
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithWaitAll() *ParallelBuilder {
	p.config.WaitCount = -1
	return p
}

// WithNodes sets the nodes to be executed in parallel.
// All nodes receive the same input and execute concurrently.
//
// Parameters:
//   - nodes: Variadic list of nodes to execute in parallel
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithNodes(nodes ...Node[any, any]) *ParallelBuilder {
	if len(nodes) > 0 {
		p.config.Nodes = nodes
	}
	return p
}

// WithAggregator sets the function that combines parallel results into final output.
// The aggregator is called after the wait condition is satisfied.
//
// Parameters:
//   - aggregator: Function that receives:
//   - ctx: Context for cancellation
//   - results: Slice of results from completed nodes (may contain nil for incomplete/failed nodes)
//     Returns the aggregated final output.
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *ParallelBuilder {
	if aggregator != nil {
		p.config.Aggregator = aggregator
	}
	return p
}

// WithCancelRemaining configures the parallel node to cancel remaining nodes
// after the wait condition is satisfied (e.g., after WaitCount completions).
// This saves resources but prevents remaining nodes from producing results.
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithCancelRemaining() *ParallelBuilder {
	p.config.CancelRemaining = true
	return p
}

// WithContinueOnError configures the parallel node to continue waiting for other nodes
// when one fails. By default, the first error causes immediate termination.
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithContinueOnError() *ParallelBuilder {
	p.config.ContinueOnError = true
	return p
}

// WithRequiredSuccesses sets the minimum number of successful completions needed.
// If fewer than this number succeed, the parallel node returns an error.
//
// Parameters:
//   - requiredSuccesses: Minimum successful completions required
//
// Returns the builder for method chaining.
func (p *ParallelBuilder) WithRequiredSuccesses(requiredSuccesses int) *ParallelBuilder {
	p.config.RequiredSuccesses = requiredSuccesses
	return p
}

// build constructs the final Parallel node from the accumulated configuration.
// This is called internally by Builder.Parallel() after the configuration closure executes.
//
// Returns:
//   - The constructed Parallel node
//   - Error if configuration is invalid or build was already called
func (p *ParallelBuilder) build() (Node[any, any], error) {
	// Ensure build is called only once
	if !p.once.markBuilt() {
		return nil, errors.New("parallel already built: build() can only be called once")
	}

	// Delegate to Parallel constructor for validation and creation
	return NewParallel(p.config)
}

// ==================== Builder (Main) ====================

// Builder provides a fluent API for constructing complex workflows.
// It accumulates nodes in a sequential chain and validates the complete flow when building.
// Once Build() is called, the builder becomes immutable.
//
// Builder supports two styles of node configuration:
// 1. Direct node addition via Then()
// 2. Scoped configuration via closure-based methods (Loop, Branch, Batch, Async, Parallel)
//
// Example:
//
//	flow, err := NewBuilder().
//	    Then(validateNode).
//	    Loop(func(loop *LoopBuilder) {
//	        loop.WithNode(processNode).
//	            WithMaxIterations(10)
//	    }).
//	    Branch(func(branch *BranchBuilder) {
//	        branch.WithNode(decisionNode).
//	            WithBranch("success", successNode).
//	            WithBranch("failure", failureNode).
//	            WithBranchResolver(resolver)
//	    }).
//	    Then(finalNode).
//	    Build()
type Builder struct {
	errs  []error          // Accumulated errors from configuration
	nodes []Node[any, any] // Sequential chain of nodes
	once  buildOnce        // Ensures single build
}

// NewBuilder creates a new Builder instance for constructing workflows.
// The builder starts empty and nodes can be added through Then() or
// specialized builder methods.
//
// Returns a new Builder ready for configuration.
func NewBuilder() *Builder {
	return &Builder{}
}

// validate checks if the builder state is valid and ready to build a flow.
// It checks for accumulated configuration errors and ensures at least one node exists.
//
// Returns:
//   - nil if validation passes
//   - Combined error if any configuration errors occurred
//   - Error if no nodes were added to the flow
func (b *Builder) validate() error {
	// Check for accumulated configuration errors
	if len(b.errs) != 0 {
		return errors.Join(b.errs...)
	}

	// Ensure flow contains at least one node
	if len(b.nodes) == 0 {
		return errors.New("flow must contain at least one node: current flow is empty")
	}

	return nil
}

// recordError stores an error to be returned during validation.
// Nil errors are silently ignored. Multiple errors are accumulated
// and later combined via errors.Join() in validate().
//
// Parameters:
//   - err: Error to record (nil is ignored)
func (b *Builder) recordError(err error) {
	if err == nil {
		return
	}
	b.errs = append(b.errs, err)
}

// Then adds a node to the sequential flow chain.
// Nodes are executed in the order they are added.
// Nil nodes are silently ignored for convenience.
//
// Parameters:
//   - node: Node to add to the flow chain
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Then(node1).Then(node2).Then(node3)
func (b *Builder) Then(node Node[any, any]) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Silently ignore nil nodes
	if node != nil {
		b.nodes = append(b.nodes, node)
	}
	return b
}

// Loop creates and configures a loop node through a closure.
// The closure receives a LoopBuilder for configuration, which is automatically
// built and added to the flow after the closure executes.
//
// Parameters:
//   - config: Closure that receives a LoopBuilder for configuration
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Loop(func(loop *LoopBuilder) {
//	    loop.WithNode(retryNode).
//	        WithMaxIterations(3).
//	        WithTerminator(func(ctx context.Context, i int, input, output any) (bool, error) {
//	            return output != nil, nil
//	        })
//	})
func (b *Builder) Loop(config func(*LoopBuilder)) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Create loop builder
	lb := newLoopBuilder(b)

	// Execute configuration closure
	config(lb)

	// Build and add the loop node
	loop, err := lb.build()
	if err != nil {
		b.recordError(err)
	} else {
		b.nodes = append(b.nodes, loop)
	}

	return b
}

// Branch creates and configures a branch node through a closure.
// The closure receives a BranchBuilder for configuration, which is automatically
// built and added to the flow after the closure executes.
//
// Parameters:
//   - config: Closure that receives a BranchBuilder for configuration
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Branch(func(branch *BranchBuilder) {
//	    branch.WithNode(decisionNode).
//	        WithBranch("high", optimizeNode).
//	        WithBranch("low", defaultNode).
//	        WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
//	            priority := output.(int)
//	            if priority > 5 {
//	                return "high", nil
//	            }
//	            return "low", nil
//	        })
//	})
func (b *Builder) Branch(config func(*BranchBuilder)) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Create branch builder
	bb := newBranchBuilder(b)

	// Execute configuration closure
	config(bb)

	// Build and add the branch node
	branch, err := bb.build()
	if err != nil {
		b.recordError(err)
	} else {
		b.nodes = append(b.nodes, branch)
	}

	return b
}

// Batch creates and configures a batch processing node through a closure.
// The closure receives a BatchBuilder for configuration, which is automatically
// built and added to the flow after the closure executes.
//
// Parameters:
//   - config: Closure that receives a BatchBuilder for configuration
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Batch(func(batch *BatchBuilder) {
//	    batch.WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
//	            data := input.([]int)
//	            segments := make([]any, len(data))
//	            for i, v := range data {
//	                segments[i] = v
//	            }
//	            return segments, nil
//	        }).
//	        WithNode(processSegmentNode).
//	        WithConcurrencyLimit(5).
//	        WithAggregator(func(ctx context.Context, results []any) (any, error) {
//	            sum := 0
//	            for _, r := range results {
//	                sum += r.(int)
//	            }
//	            return sum, nil
//	        })
//	})
func (b *Builder) Batch(config func(*BatchBuilder)) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Create batch builder
	bb := newBatchBuilder(b)

	// Execute configuration closure
	config(bb)

	// Build and add the batch node
	batch, err := bb.build()
	if err != nil {
		b.recordError(err)
	} else {
		b.nodes = append(b.nodes, batch)
	}

	return b
}

// Async creates and configures an async execution node through a closure.
// The closure receives an AsyncBuilder for configuration, which is automatically
// built and added to the flow after the closure executes.
//
// Parameters:
//   - config: Closure that receives an AsyncBuilder for configuration
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Async(func(async *AsyncBuilder) {
//	    async.WithNode(longRunningNode).
//	        WithPool(workerPool)
//	})
func (b *Builder) Async(config func(*AsyncBuilder)) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Create async builder
	ab := newAsyncBuilder(b)

	// Execute configuration closure
	config(ab)

	// Build and add the async node
	async, err := ab.build()
	if err != nil {
		b.recordError(err)
	} else {
		b.nodes = append(b.nodes, async)
	}

	return b
}

// Parallel creates and configures a parallel execution node through a closure.
// The closure receives a ParallelBuilder for configuration, which is automatically
// built and added to the flow after the closure executes.
//
// Parameters:
//   - config: Closure that receives a ParallelBuilder for configuration
//
// Returns the builder for method chaining.
//
// Example:
//
//	builder.Parallel(func(parallel *ParallelBuilder) {
//	    parallel.WithNodes(serviceA, serviceB, serviceC).
//	        WithWaitAny().
//	        WithCancelRemaining().
//	        WithAggregator(func(ctx context.Context, results []any) (any, error) {
//	            // Return first non-nil result
//	            for _, r := range results {
//	                if r != nil {
//	                    return r, nil
//	                }
//	            }
//	            return nil, errors.New("all results are nil")
//	        })
//	})
func (b *Builder) Parallel(config func(*ParallelBuilder)) *Builder {
	// Prevent modification after build
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	// Create parallel builder
	pb := newParallelBuilder(b)

	// Execute configuration closure
	config(pb)

	// Build and add the parallel node
	parallel, err := pb.build()
	if err != nil {
		b.recordError(err)
	} else {
		b.nodes = append(b.nodes, parallel)
	}

	return b
}

// Build validates the accumulated configuration and constructs the final Flow node.
// This method can only be called once - subsequent calls will return an error.
// After Build() is called, the builder becomes immutable and cannot be modified.
//
// Returns:
//   - The constructed Flow node containing all configured nodes
//   - Error if validation fails, no nodes were added, or Build() was already called
//
// Example:
//
//	flow, err := NewBuilder().
//	    Then(node1).
//	    Loop(func(loop *LoopBuilder) {
//	        loop.WithNode(node2).WithMaxIterations(5)
//	    }).
//	    Then(node3).
//	    Build()
//	if err != nil {
//	    return fmt.Errorf("failed to build flow: %w", err)
//	}
//	result, err := flow.Run(ctx, input)
func (b *Builder) Build() (Node[any, any], error) {
	// Ensure build is called only once
	if !b.once.markBuilt() {
		return nil, errors.New("builder already built: Build() can only be called once")
	}

	// Validate configuration
	if err := b.validate(); err != nil {
		return nil, err
	}

	// Construct and return the flow
	return NewFlow(b.nodes...)
}
