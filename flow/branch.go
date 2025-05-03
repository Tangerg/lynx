package flow

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// Branch enables conditional execution based on the output of a processor.
// It routes processing through different nodes based on a comparable identifier.
type Branch[T comparable] struct {
	// processor is the node that generates the output used for branch selection
	processor Processor[any, any]
	// branchResolver determines which branch to take based on input and processor output
	branchResolver func(context.Context, any, any) (T, error)
	// branches maps branch identifiers to their corresponding processing nodes
	branches map[T]Node[any, any]
}

// resolveBranch determines which branch to execute based on input and processor output.
// Returns the selected Node and any error that occurred during branch selection.
func (b *Branch[T]) resolveBranch(ctx context.Context, input any, output any) (Node[any, any], error) {
	err := b.processor.checkContextCancellation(ctx)
	if err != nil {
		return nil, err
	}
	branch, err := b.branchResolver(ctx, input, output)
	if err != nil {
		return nil, err
	}
	node, ok := b.branches[branch]
	if !ok {
		return nil, fmt.Errorf("branch '%v' not found: available branches are %v", branch, slices.Collect(maps.Keys(b.branches)))
	}
	return node, nil
}

// run executes the branch processor and then the appropriate branch based on its output.
// If no branches are defined or no branch resolver is set, it only runs the processor.
func (b *Branch[T]) run(ctx context.Context, input any) (output any, err error) {
	output, err = b.processor.Run(ctx, input)
	if err != nil {
		return
	}
	if len(b.branches) == 0 || b.branchResolver == nil {
		return
	}
	branch, err := b.resolveBranch(ctx, input, output)
	if err != nil {
		return
	}
	return branch.Run(ctx, output)
}

// Run implements the Node interface for Branch.
// It first validates the processor, then executes the branch logic.
func (b *Branch[T]) Run(ctx context.Context, input any) (any, error) {
	err := validateProcessor(b.processor)
	if err != nil {
		return nil, err
	}
	return b.run(ctx, input)
}

// AddBranch adds a new branch with the given identifier and processing node.
// Initializes the branches map if needed. Ignores nil nodes.
// Returns the Branch for method chaining.
func (b *Branch[T]) AddBranch(branch T, node Node[any, any]) *Branch[T] {
	if b.branches == nil {
		b.branches = make(map[T]Node[any, any])
	}
	if node != nil {
		b.branches[branch] = node
	}
	return b
}

// WithProcessor sets the processor for this branch.
// The processor's output will be used to determine which branch to execute.
// Returns the Branch for method chaining.
func (b *Branch[T]) WithProcessor(processor Processor[any, any]) *Branch[T] {
	b.processor = processor
	return b
}

// BranchBuilder helps construct a Branch node with a fluent API.
// It maintains references to both the parent flow and the branch being built.
type BranchBuilder struct {
	flow   *Flow
	branch *Branch[string]
}

// AddBranch adds a new branch to the branch being built.
// Returns the BranchBuilder for method chaining.
func (b *BranchBuilder) AddBranch(branch string, node Node[any, any]) *BranchBuilder {
	b.branch.AddBranch(branch, node)
	return b
}

// WithProcessor sets the processor for the branch being built.
// Returns the BranchBuilder for method chaining.
func (b *BranchBuilder) WithProcessor(processor Processor[any, any]) *BranchBuilder {
	b.branch.WithProcessor(processor)
	return b
}

// Then adds the constructed branch to the parent flow and returns the flow.
// This completes the branch construction and continues building the flow.
func (b *BranchBuilder) Then() *Flow {
	b.flow.Then(b.branch)
	return b.flow
}
