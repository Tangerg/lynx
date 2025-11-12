package flow

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/Tangerg/lynx/pkg/sync"
)

// buildOnce provides atomic build-once semantics using lock-free operations.
type buildOnce struct {
	state atomic.Bool
}

// markBuilt atomically marks as built and returns true if this is the first call.
func (b *buildOnce) markBuilt() bool {
	return b.state.CompareAndSwap(false, true)
}

// isBuilt checks if already built.
func (b *buildOnce) isBuilt() bool {
	return b.state.Load()
}

// LoopBuilder provides a fluent API for constructing Loop nodes.
// It accumulates configuration and validates it when building the loop.
// Once Then() is called, the builder becomes immutable.
type LoopBuilder struct {
	builder *Builder
	config  *LoopConfig[any, any]
	once    buildOnce
}

// NewLoopBuilder creates a new LoopBuilder instance.
// If a Builder is provided, the loop will be added to that builder's chain.
// Otherwise, a new Builder is created.
func NewLoopBuilder(builders ...*Builder) *LoopBuilder {
	lb := &LoopBuilder{
		config: &LoopConfig[any, any]{},
	}

	if len(builders) > 0 {
		lb.builder = builders[0]
	} else {
		lb.builder = NewBuilder()
	}

	return lb
}

// WithNode sets the node to be executed in each iteration.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (l *LoopBuilder) WithNode(node Node[any, any]) *LoopBuilder {
	if l.once.isBuilt() {
		l.builder.recordError(errors.New("cannot modify loop: already built"))
		return l
	}

	if node != nil {
		l.config.Node = node
	}
	return l
}

// WithMaxIterations sets the maximum number of iterations allowed.
// Provides a hard limit to prevent infinite loops.
//
// Parameters:
//   - maxIterations: Maximum iterations (must be > 0 to take effect, <= 0 is ignored)
//
// If called after Then(), records an error and returns without modification.
func (l *LoopBuilder) WithMaxIterations(maxIterations int) *LoopBuilder {
	if l.once.isBuilt() {
		l.builder.recordError(errors.New("cannot modify loop: already built"))
		return l
	}

	if maxIterations > 0 {
		l.config.MaxIterations = maxIterations
	}
	return l
}

// WithTerminator sets the function that determines when to stop looping.
// The terminator receives:
//   - ctx: Context for cancellation
//   - iteration: Current iteration number (0-based)
//   - input: Original input to the loop
//   - output: Output from the last iteration
//
// Returns true to continue looping, false to stop.
// If called after Then(), records an error and returns without modification.
func (l *LoopBuilder) WithTerminator(terminator func(context.Context, int, any, any) (bool, error)) *LoopBuilder {
	if l.once.isBuilt() {
		l.builder.recordError(errors.New("cannot modify loop: already built"))
		return l
	}

	if terminator != nil {
		l.config.Terminator = terminator
	}
	return l
}

// Then builds the loop and adds it to the parent builder's node chain.
// Returns the parent builder for continued flow construction.
// Can only be called once - subsequent calls will record an error.
func (l *LoopBuilder) Then() *Builder {
	if !l.once.markBuilt() {
		l.builder.recordError(errors.New("loop already built: Then() can only be called once"))
		return l.builder
	}

	loop, err := NewLoop(l.config)
	if err != nil {
		l.builder.recordError(err)
	}
	l.builder.Then(loop)
	return l.builder
}

// BranchBuilder provides a fluent API for constructing Branch nodes.
// It accumulates branch configuration and validates it when building.
// Once Then() is called, the builder becomes immutable.
type BranchBuilder struct {
	builder *Builder
	config  *BranchConfig
	once    buildOnce
}

// NewBranchBuilder creates a new BranchBuilder instance.
// If a Builder is provided, the branch will be added to that builder's chain.
// Otherwise, a new Builder is created.
func NewBranchBuilder(builders ...*Builder) *BranchBuilder {
	bb := &BranchBuilder{
		config: &BranchConfig{
			Branches: make(map[string]Node[any, any]),
		},
	}

	if len(builders) > 0 {
		bb.builder = builders[0]
	} else {
		bb.builder = NewBuilder()
	}

	return bb
}

// WithNode sets the main decision node whose output determines which branch to take.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BranchBuilder) WithNode(node Node[any, any]) *BranchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify branch: already built"))
		return b
	}

	if node != nil {
		b.config.Node = node
	}
	return b
}

// WithBranch adds a single branch mapping from name to node.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BranchBuilder) WithBranch(branchName string, node Node[any, any]) *BranchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify branch: already built"))
		return b
	}

	if node != nil {
		b.config.Branches[branchName] = node
	}
	return b
}

// WithBranches sets all branches at once, replacing any previously configured branches.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BranchBuilder) WithBranches(branches map[string]Node[any, any]) *BranchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify branch: already built"))
		return b
	}

	if branches != nil {
		b.config.Branches = branches
	}
	return b
}

// WithBranchResolver sets the function that determines which branch to execute.
// The resolver receives:
//   - ctx: Context for cancellation
//   - input: Original input to the main node
//   - output: Output from the main node
//
// Returns the name of the branch to execute.
// If called after Then(), records an error and returns without modification.
func (b *BranchBuilder) WithBranchResolver(resolver func(context.Context, any, any) (string, error)) *BranchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify branch: already built"))
		return b
	}

	if resolver != nil {
		b.config.BranchResolver = resolver
	}
	return b
}

// Then builds the branch and adds it to the parent builder's node chain.
// Returns the parent builder for continued flow construction.
// Can only be called once - subsequent calls will record an error.
func (b *BranchBuilder) Then() *Builder {
	if !b.once.markBuilt() {
		b.builder.recordError(errors.New("branch already built: Then() can only be called once"))
		return b.builder
	}

	branch, err := NewBranch(b.config)
	if err != nil {
		b.builder.recordError(err)
	}
	b.builder.Then(branch)
	return b.builder
}

// BatchBuilder provides a fluent API for constructing Batch nodes.
// It accumulates batch processing configuration and validates it when building.
// Once Then() is called, the builder becomes immutable.
type BatchBuilder struct {
	builder *Builder
	config  *BatchConfig[any, any, any, any]
	once    buildOnce
}

// NewBatchBuilder creates a new BatchBuilder instance.
// If a Builder is provided, the batch will be added to that builder's chain.
// Otherwise, a new Builder is created.
func NewBatchBuilder(builders ...*Builder) *BatchBuilder {
	bb := &BatchBuilder{
		config: &BatchConfig[any, any, any, any]{},
	}

	if len(builders) > 0 {
		bb.builder = builders[0]
	} else {
		bb.builder = NewBuilder()
	}

	return bb
}

// WithContinueOnError configures the batch to continue processing remaining segments
// even when one fails. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BatchBuilder) WithContinueOnError() *BatchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify batch: already built"))
		return b
	}

	b.config.ContinueOnError = true
	return b
}

// WithConcurrencyLimit sets the maximum number of concurrent segment processors.
// Must be > 0 to take effect. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BatchBuilder) WithConcurrencyLimit(concurrencyLimit int) *BatchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify batch: already built"))
		return b
	}

	if concurrencyLimit > 0 {
		b.config.ConcurrencyLimit = concurrencyLimit
	}
	return b
}

// WithNode sets the node to process each segment.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BatchBuilder) WithNode(node Node[any, any]) *BatchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify batch: already built"))
		return b
	}

	if node != nil {
		b.config.Node = node
	}
	return b
}

// WithSegmenter sets the function that splits input into segments.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BatchBuilder) WithSegmenter(segmenter func(context.Context, any) ([]any, error)) *BatchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify batch: already built"))
		return b
	}

	if segmenter != nil {
		b.config.Segmenter = segmenter
	}
	return b
}

// WithAggregator sets the function that combines segment results into final output.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (b *BatchBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *BatchBuilder {
	if b.once.isBuilt() {
		b.builder.recordError(errors.New("cannot modify batch: already built"))
		return b
	}

	if aggregator != nil {
		b.config.Aggregator = aggregator
	}
	return b
}

// Then builds the batch and adds it to the parent builder's node chain.
// Returns the parent builder for continued flow construction.
// Can only be called once - subsequent calls will record an error.
func (b *BatchBuilder) Then() *Builder {
	if !b.once.markBuilt() {
		b.builder.recordError(errors.New("batch already built: Then() can only be called once"))
		return b.builder
	}

	batch, err := NewBatch(b.config)
	if err != nil {
		b.builder.recordError(err)
	}
	b.builder.Then(batch)
	return b.builder
}

// AsyncBuilder provides a fluent API for constructing Async nodes.
// It accumulates async execution configuration and validates it when building.
// Once Then() is called, the builder becomes immutable.
type AsyncBuilder struct {
	builder *Builder
	config  *AsyncConfig[any, any]
	once    buildOnce
}

// NewAsyncBuilder creates a new AsyncBuilder instance.
// If a Builder is provided, the async node will be added to that builder's chain.
// Otherwise, a new Builder is created.
func NewAsyncBuilder(builders ...*Builder) *AsyncBuilder {
	ab := &AsyncBuilder{
		config: &AsyncConfig[any, any]{},
	}

	if len(builders) > 0 {
		ab.builder = builders[0]
	} else {
		ab.builder = NewBuilder()
	}

	return ab
}

// WithNode sets the node to be executed asynchronously.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (a *AsyncBuilder) WithNode(node Node[any, any]) *AsyncBuilder {
	if a.once.isBuilt() {
		a.builder.recordError(errors.New("cannot modify async: already built"))
		return a
	}

	if node != nil {
		a.config.Node = node
	}
	return a
}

// WithPool sets the thread pool for async execution.
// If not set, a default no-pool executor will be used.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (a *AsyncBuilder) WithPool(pool sync.Pool) *AsyncBuilder {
	if a.once.isBuilt() {
		a.builder.recordError(errors.New("cannot modify async: already built"))
		return a
	}

	if pool != nil {
		a.config.Pool = pool
	}
	return a
}

// Then builds the async node and adds it to the parent builder's node chain.
// Returns the parent builder for continued flow construction.
// Can only be called once - subsequent calls will record an error.
func (a *AsyncBuilder) Then() *Builder {
	if !a.once.markBuilt() {
		a.builder.recordError(errors.New("async already built: Then() can only be called once"))
		return a.builder
	}

	async, err := NewAsync(a.config)
	if err != nil {
		a.builder.recordError(err)
	}
	a.builder.Then(async)
	return a.builder
}

// ParallelBuilder provides a fluent API for constructing Parallel nodes.
// It accumulates parallel execution configuration and validates it when building.
// Once Then() is called, the builder becomes immutable.
type ParallelBuilder struct {
	builder *Builder
	config  *ParallelConfig[any, any]
	once    buildOnce
}

// NewParallelBuilder creates a new ParallelBuilder instance.
// If a Builder is provided, the parallel node will be added to that builder's chain.
// Otherwise, a new Builder is created.
func NewParallelBuilder(builders ...*Builder) *ParallelBuilder {
	pb := &ParallelBuilder{
		config: &ParallelConfig[any, any]{},
	}

	if len(builders) > 0 {
		pb.builder = builders[0]
	} else {
		pb.builder = NewBuilder()
	}

	return pb
}

// WithWaitCount sets how many node completions to wait for before aggregating.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithWaitCount(waitCount int) *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.WaitCount = waitCount
	return p
}

// WithWaitAny configures the parallel node to wait for only the first completion.
// Shorthand for WithWaitCount(1). Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithWaitAny() *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.WaitCount = 1
	return p
}

// WithWaitAll configures the parallel node to wait for all nodes to complete.
// This is the default behavior. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithWaitAll() *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.WaitCount = -1
	return p
}

// WithNodes sets the nodes to be executed in parallel.
// All nodes receive the same input. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithNodes(nodes ...Node[any, any]) *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	if len(nodes) > 0 {
		p.config.Nodes = nodes
	}
	return p
}

// WithAggregator sets the function that combines parallel results into final output.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithAggregator(aggregator func(context.Context, []any) (any, error)) *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	if aggregator != nil {
		p.config.Aggregator = aggregator
	}
	return p
}

// WithCancelRemaining configures the parallel node to cancel remaining nodes
// after WaitCount is reached. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithCancelRemaining() *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.CancelRemaining = true
	return p
}

// WithContinueOnError configures the parallel node to continue waiting for other nodes
// when one fails. Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithContinueOnError() *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.ContinueOnError = true
	return p
}

// WithRequiredSuccesses sets the minimum number of successful completions needed.
// Returns the builder for method chaining.
// If called after Then(), records an error and returns without modification.
func (p *ParallelBuilder) WithRequiredSuccesses(requiredSuccesses int) *ParallelBuilder {
	if p.once.isBuilt() {
		p.builder.recordError(errors.New("cannot modify parallel: already built"))
		return p
	}

	p.config.RequiredSuccesses = requiredSuccesses
	return p
}

// Then builds the parallel node and adds it to the parent builder's node chain.
// Returns the parent builder for continued flow construction.
// Can only be called once - subsequent calls will record an error.
func (p *ParallelBuilder) Then() *Builder {
	if !p.once.markBuilt() {
		p.builder.recordError(errors.New("parallel already built: Then() can only be called once"))
		return p.builder
	}

	parallel, err := NewParallel(p.config)
	if err != nil {
		p.builder.recordError(err)
	}
	p.builder.Then(parallel)
	return p.builder
}

// Builder provides a fluent API for constructing complex workflows.
// It accumulates nodes in a sequential chain and validates the complete flow when building.
// Once Build() is called, the builder becomes immutable.
//
// Example:
//
//	flow, err := NewBuilder().
//	    Then(validateNode).
//	    Then(processNode).
//	    Branch().
//	        WithNode(decisionNode).
//	        WithBranch("success", successNode).
//	        WithBranch("failure", failureNode).
//	        WithBranchResolver(resolver).
//	        Then().
//	    Then(finalNode).
//	    Build()
type Builder struct {
	errs  []error
	nodes []Node[any, any]
	once  buildOnce
}

// NewBuilder creates a new Builder instance for constructing workflows.
func NewBuilder() *Builder {
	return &Builder{}
}

// validate checks if the builder state is valid and ready to build a flow.
// Returns an error if any configuration errors were recorded or if no nodes were added.
func (b *Builder) validate() error {
	if len(b.errs) != 0 {
		return errors.Join(b.errs...)
	}

	if len(b.nodes) == 0 {
		return errors.New("flow must contain at least one node: current flow is empty")
	}

	return nil
}

// recordError stores an error to be returned during validation.
// Nil errors are ignored.
func (b *Builder) recordError(err error) {
	if err == nil {
		return
	}
	b.errs = append(b.errs, err)
}

// Then adds a node to the sequential flow chain.
// Nil nodes are ignored. Returns the builder for method chaining.
// If called after Build(), records an error and returns without modification.
func (b *Builder) Then(node Node[any, any]) *Builder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
		return b
	}

	if node != nil {
		b.nodes = append(b.nodes, node)
	}
	return b
}

// Branch creates a new BranchBuilder for constructing a conditional branch node.
// Returns a BranchBuilder that can be configured and added back to this builder.
// If called after Build(), records an error.
func (b *Builder) Branch() *BranchBuilder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
	}
	return NewBranchBuilder(b)
}

// Loop creates a new LoopBuilder for constructing an iterative loop node.
// Returns a LoopBuilder that can be configured and added back to this builder.
// If called after Build(), records an error.
func (b *Builder) Loop() *LoopBuilder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
	}
	return NewLoopBuilder(b)
}

// Batch creates a new BatchBuilder for constructing a batch processing node.
// Returns a BatchBuilder that can be configured and added back to this builder.
// If called after Build(), records an error.
func (b *Builder) Batch() *BatchBuilder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
	}
	return NewBatchBuilder(b)
}

// Async creates a new AsyncBuilder for constructing an asynchronous execution node.
// Returns an AsyncBuilder that can be configured and added back to this builder.
// If called after Build(), records an error.
func (b *Builder) Async() *AsyncBuilder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
	}
	return NewAsyncBuilder(b)
}

// Parallel creates a new ParallelBuilder for constructing a parallel execution node.
// Returns a ParallelBuilder that can be configured and added back to this builder.
// If called after Build(), records an error.
func (b *Builder) Parallel() *ParallelBuilder {
	if b.once.isBuilt() {
		b.recordError(errors.New("cannot modify builder: flow already built"))
	}
	return NewParallelBuilder(b)
}

// Build validates the accumulated configuration and constructs the final Flow node.
// Returns an error if validation fails or if no nodes were added.
// Can only be called once - subsequent calls will return an error.
//
// Example:
//
//	flow, err := NewBuilder().
//	    Then(node1).
//	    Then(node2).
//	    Build()
//	if err != nil {
//	    return err
//	}
//	result, err := flow.Run(ctx, input)
func (b *Builder) Build() (Node[any, any], error) {
	if !b.once.markBuilt() {
		return nil, errors.New("builder already built: Build() can only be called once")
	}

	if err := b.validate(); err != nil {
		return nil, err
	}

	return NewFlow(b.nodes...)
}
