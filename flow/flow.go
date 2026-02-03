package flow

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// BranchBuilder provides a fluent interface for constructing branch nodes.
// It allows dynamic configuration of branches and their resolution logic.
// For type-safe branches with compile-time guarantees, use NewBranch directly with BranchConfig.
type BranchBuilder[I, O any] struct {
	config BranchConfig[I, O]
}

// NewBranchBuilder creates a new builder for constructing branch nodes.
func NewBranchBuilder[I, O any]() *BranchBuilder[I, O] {
	return &BranchBuilder[I, O]{
		config: BranchConfig[I, O]{},
	}
}

// WithBranches sets the available branches for the node.
// Each branch is identified by a unique name (map key) and associated with a processor function.
func (b *BranchBuilder[I, O]) WithBranches(branches map[string]func(context.Context, I) (O, error)) *BranchBuilder[I, O] {
	b.config.Branches = maps.Clone(branches)
	return b
}

// WithBranchResolver sets the function that determines which branch to execute based on input.
func (b *BranchBuilder[I, O]) WithBranchResolver(branchResolver func(context.Context, I) string) *BranchBuilder[I, O] {
	b.config.BranchResolver = branchResolver
	return b
}

// Build constructs the branch node from the configured settings.
// Returns an error if the configuration is invalid (e.g., missing branches or resolver).
func (b *BranchBuilder[I, O]) Build() (*Branch[I, O], error) {
	branch, err := NewBranch[I, O](b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build branch: %w", err)
	}

	return branch, nil
}

// IterationBuilder provides a fluent interface for constructing iteration nodes.
// It enables processing of collections with configurable concurrency and error handling.
// For type-safe iterations with compile-time guarantees, use NewIteration directly with IterationConfig.
type IterationBuilder[I, O any] struct {
	config IterationConfig[I, O]
}

// NewIterationBuilder creates a new builder for constructing iteration nodes.
func NewIterationBuilder[I, O any]() *IterationBuilder[I, O] {
	return &IterationBuilder[I, O]{
		config: IterationConfig[I, O]{},
	}
}

// WithProcessor sets the processing function that will be applied to each element.
// The processor receives the element index and value, returning the transformed output.
func (b *IterationBuilder[I, O]) WithProcessor(processor func(context.Context, int, I) (output O, err error)) *IterationBuilder[I, O] {
	b.config.Processor = processor
	return b
}

// WithContinueOnError determines whether to continue processing remaining elements after an error.
// If false (default), the iteration stops at the first error.
func (b *IterationBuilder[I, O]) WithContinueOnError(continueOnError bool) *IterationBuilder[I, O] {
	b.config.ContinueOnError = continueOnError
	return b
}

// WithConcurrencyLimit sets the maximum number of elements to process concurrently.
// Use 0 or negative value for unlimited concurrency (not recommended for large collections).
func (b *IterationBuilder[I, O]) WithConcurrencyLimit(concurrencyLimit int) *IterationBuilder[I, O] {
	b.config.ConcurrencyLimit = concurrencyLimit
	return b
}

// Build constructs the iteration node from the configured settings.
// Returns an error if the configuration is invalid (e.g., missing processor).
func (b *IterationBuilder[I, O]) Build() (*Iteration[I, O], error) {
	iteration, err := NewIteration(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build iteration: %w", err)
	}

	return iteration, nil
}

// LoopBuilder provides a fluent interface for constructing loop nodes.
// It enables repeated processing until a termination condition is met or max iterations reached.
// For type-safe loops with compile-time guarantees, use NewLoop directly with LoopConfig.
type LoopBuilder[T any] struct {
	config LoopConfig[T]
}

// NewLoopBuilder creates a new builder for constructing loop nodes.
func NewLoopBuilder[T any]() *LoopBuilder[T] {
	return &LoopBuilder[T]{
		config: LoopConfig[T]{},
	}
}

// WithMaxIterations sets the maximum number of iterations to prevent infinite loops.
// The loop will terminate after this many iterations even if the done condition is not met.
func (b *LoopBuilder[T]) WithMaxIterations(maxIterations int) *LoopBuilder[T] {
	b.config.MaxIterations = maxIterations
	return b
}

// WithProcessor sets the processing function executed in each iteration.
// The processor receives the current iteration count and input, returning:
//   - output: the transformed value passed to the next iteration
//   - done: whether to terminate the loop
//   - err: any error that occurred
func (b *LoopBuilder[T]) WithProcessor(
	processor func(ctx context.Context, iteration int, input T) (output T, done bool, err error),
) *LoopBuilder[T] {
	b.config.Processor = processor
	return b
}

// Build constructs the loop node from the configured settings.
// Returns an error if the configuration is invalid (e.g., missing processor or invalid max iterations).
func (b *LoopBuilder[T]) Build() (*Loop[T], error) {
	loop, err := NewLoop(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build loop: %w", err)
	}

	return loop, nil
}

// ParallelBuilder provides a fluent interface for constructing parallel nodes.
// It enables concurrent execution of multiple processors with the same input.
// For type-safe parallel processing with compile-time guarantees, use NewParallel directly with ParallelConfig.
type ParallelBuilder[I, O any] struct {
	config ParallelConfig[I, O]
}

// NewParallelBuilder creates a new builder for constructing parallel nodes.
func NewParallelBuilder[I, O any]() *ParallelBuilder[I, O] {
	return &ParallelBuilder[I, O]{
		config: ParallelConfig[I, O]{},
	}
}

// WithProcessors sets the processors to execute concurrently.
// Each processor receives the same input and operates independently.
func (b *ParallelBuilder[I, O]) WithProcessors(processors []func(context.Context, I) (O, error)) *ParallelBuilder[I, O] {
	b.config.Processors = slices.Clone(processors)
	return b
}

// WithConcurrencyLimit sets the maximum number of processors to run concurrently.
// Use 0 or negative value for unlimited concurrency (executes all processors at once).
func (b *ParallelBuilder[I, O]) WithConcurrencyLimit(concurrencyLimit int) *ParallelBuilder[I, O] {
	b.config.ConcurrencyLimit = concurrencyLimit
	return b
}

// WithContinueOnError determines whether to continue executing remaining processors after an error.
// If false (default), the parallel execution stops at the first error.
func (b *ParallelBuilder[I, O]) WithContinueOnError(continueOnError bool) *ParallelBuilder[I, O] {
	b.config.ContinueOnError = continueOnError
	return b
}

// Build constructs the parallel node from the configured settings.
// Returns an error if the configuration is invalid (e.g., no processors defined).
func (b *ParallelBuilder[I, O]) Build() (*Parallel[I, O], error) {
	parallel, err := NewParallel(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build parallel: %w", err)
	}

	return parallel, nil
}

// Flow provides a fluent interface for building complex workflows by chaining nodes together.
// It accumulates nodes and any configuration errors during the build process,
// validating everything before constructing the final pipeline.
//
// Flow uses dynamic typing (any) for maximum flexibility. For type-safe pipelines
// with compile-time type checking, use Pipe2-Pipe10 directly.
//
// Example:
//
//	flow := NewFlow().
//		Loop(func(b *LoopBuilder[any]) {
//			b.WithMaxIterations(10).WithProcessor(...)
//		}).
//		Branch(func(b *BranchBuilder[any, any]) {
//			b.WithBranches(...).WithBranchResolver(...)
//		})
//	node, err := flow.Build()
type Flow struct {
	errors []error
	nodes  []Node[any, any]
}

// NewFlow creates a new empty flow builder.
func NewFlow() *Flow {
	return &Flow{}
}

// append adds a node to the flow, accumulating any errors that occur during node creation.
// This is an internal helper method used by the public builder methods.
func (f *Flow) append(node Node[any, any], err error) *Flow {
	if err != nil {
		f.errors = append(f.errors, err)
		return f
	}

	if node == nil {
		f.errors = append(f.errors, errors.New("node is nil"))
		return f
	}

	f.nodes = append(f.nodes, node)
	return f
}

// Then adds a custom node to the flow.
// Nil nodes are silently ignored without generating errors.
func (f *Flow) Then(node Node[any, any]) *Flow {
	if node != nil {
		f.nodes = append(f.nodes, node)
	}
	return f
}

// Loop adds a loop node to the flow using a builder configuration function.
// Configuration errors are accumulated and reported during Build().
//
// Example:
//
//	flow.Loop(func(b *LoopBuilder[any]) {
//		b.WithMaxIterations(5).WithProcessor(func(ctx context.Context, i int, v any) (any, bool, error) {
//			// process and return (result, shouldStop, error)
//		})
//	})
func (f *Flow) Loop(config func(*LoopBuilder[any])) *Flow {
	builder := NewLoopBuilder[any]()
	config(builder)

	loop, err := builder.Build()
	return f.append(loop, err)
}

// Branch adds a branch node to the flow using a builder configuration function.
// Configuration errors are accumulated and reported during Build().
//
// Example:
//
//	flow.Branch(func(b *BranchBuilder[any, any]) {
//		b.WithBranches(map[string]func(context.Context, any) (any, error){
//			"path1": processor1,
//			"path2": processor2,
//		}).WithBranchResolver(func(ctx context.Context, input any) string {
//			// return branch name based on input
//		})
//	})
func (f *Flow) Branch(config func(*BranchBuilder[any, any])) *Flow {
	builder := NewBranchBuilder[any, any]()
	config(builder)

	branch, err := builder.Build()
	return f.append(branch, err)
}

// Iteration adds an iteration node to the flow using a builder configuration function.
// The node expects input to be a slice ([]any) and returns a slice of processed results.
// Configuration errors are accumulated and reported during Build().
//
// Example:
//
//	flow.Iteration(func(b *IterationBuilder[any, any]) {
//		b.WithProcessor(func(ctx context.Context, idx int, item any) (any, error) {
//			// process each item
//		}).WithConcurrencyLimit(10)
//	})
func (f *Flow) Iteration(config func(*IterationBuilder[any, any])) *Flow {
	builder := NewIterationBuilder[any, any]()
	config(builder)

	iteration, err := builder.Build()

	var node Node[any, any]
	if iteration != nil {
		node = Func[any, any](func(ctx context.Context, input any) (any, error) {
			inputs, ok := input.([]any)
			if !ok {
				return nil, fmt.Errorf("iteration expects []any input, got %T", input)
			}

			return iteration.Run(ctx, inputs)
		})
	}

	return f.append(node, err)
}

// Parallel adds a parallel node to the flow using a builder configuration function.
// The node executes multiple processors concurrently, each receiving the same input.
// Configuration errors are accumulated and reported during Build().
//
// Example:
//
//	flow.Parallel(func(b *ParallelBuilder[any, any]) {
//		b.WithProcessors([]func(context.Context, any) (any, error){
//			processor1,
//			processor2,
//		}).WithConcurrencyLimit(2)
//	})
func (f *Flow) Parallel(config func(*ParallelBuilder[any, any])) *Flow {
	builder := NewParallelBuilder[any, any]()
	config(builder)

	parallel, err := builder.Build()

	var node Node[any, any]
	if parallel != nil {
		node = Func[any, any](func(ctx context.Context, input any) (any, error) {
			return parallel.Run(ctx, input)
		})
	}

	return f.append(node, err)
}

// validate checks if the flow is valid and ready to be built.
// It verifies that:
//   - No configuration errors occurred during node creation
//   - At least one node exists in the flow
func (f *Flow) validate() error {
	// Check for accumulated configuration errors
	if len(f.errors) > 0 {
		return fmt.Errorf("flow configuration failed: %w", errors.Join(f.errors...))
	}

	// Ensure flow contains at least one node
	if len(f.nodes) == 0 {
		return errors.New("flow must contain at least one node")
	}

	return nil
}

// Build validates the flow and constructs the final pipeline node.
// Returns an error if:
//   - Any node configuration failed during the build process
//   - The flow contains no nodes
//   - The pipeline construction fails
//
// The resulting node can be executed with Run(ctx, input).
func (f *Flow) Build() (Node[any, any], error) {
	if err := f.validate(); err != nil {
		return nil, err
	}

	pipeline, err := Pipe(f.nodes...)
	if err != nil {
		return nil, fmt.Errorf("failed to build flow pipeline: %w", err)
	}

	return pipeline, nil
}
