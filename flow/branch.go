package flow

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// BranchConfig contains the configuration for creating a Branch node.
// It defines a main node and conditional branches based on the main node's output.
type BranchConfig struct {
	// Node is the main processing unit whose output determines which branch to take
	Node Node[any, any]

	// BranchResolver determines which branch to execute based on the input and output.
	// Parameters:
	//   - ctx: Context for cancellation control
	//   - input: Original input to the main node
	//   - output: Output from the main node
	// Returns:
	//   - string: The name of the branch to execute
	//   - error: Any error during branch resolution
	// If nil, no branching occurs and only the main node is executed.
	BranchResolver func(context.Context, any, any) (string, error)

	// Branches maps branch names to their corresponding nodes.
	// The branch selected by BranchResolver will be executed with the main node's output.
	// If empty, no branching occurs.
	Branches map[string]Node[any, any]
}

// validate checks if the BranchConfig is valid and ready to use.
// Returns an error if the config or its Node field is nil.
func (cfg *BranchConfig) validate() error {
	if cfg == nil {
		return errors.New("branch config cannot be nil")
	}

	if cfg.Node == nil {
		return errors.New("branch node cannot be nil")
	}

	return nil
}

// Branch represents a conditional node that executes different paths based on a decision.
// It first runs the main node, then uses the resolver to determine and execute a branch.
//
// Execution flow:
//  1. Execute the main node with the input
//  2. Use the branch resolver to determine which branch to take (based on input and output)
//  3. Execute the selected branch node with the main node's output
type Branch struct {
	node           Node[any, any]
	branchResolver func(context.Context, any, any) (string, error)
	branches       map[string]Node[any, any]
}

// NewBranch creates a new Branch instance with the provided configuration.
// Returns an error if the configuration is invalid.
//
// Example:
//
//	branch, err := NewBranch(&BranchConfig{
//	    Node: validatorNode,
//	    BranchResolver: func(ctx context.Context, input, output any) (string, error) {
//	        if isValid(output) {
//	            return "success", nil
//	        }
//	        return "failure", nil
//	    },
//	    Branches: map[string]Node[any, any]{
//	        "success": successNode,
//	        "failure": failureNode,
//	    },
//	})
func NewBranch(cfg *BranchConfig) (*Branch, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Branch{
		node:           cfg.Node,
		branchResolver: cfg.BranchResolver,
		branches:       cfg.Branches,
	}, nil
}

// resolveBranch determines which branch node to execute based on the resolver's decision.
// Returns an error if the branch name doesn't exist in the branches map.
//
// Parameters:
//   - ctx: Context for cancellation control
//   - input: Original input to the main node
//   - output: Output from the main node
//
// Returns:
//   - The node corresponding to the resolved branch name
//   - An error if branch resolution fails or the branch doesn't exist
func (b *Branch) resolveBranch(ctx context.Context, input, output any) (Node[any, any], error) {
	branchName, err := b.branchResolver(ctx, input, output)
	if err != nil {
		return nil, err
	}

	branchNode, exists := b.branches[branchName]
	if !exists {
		availableBranches := slices.Collect(maps.Keys(b.branches))
		return nil, fmt.Errorf(
			"branch '%s' not found: available branches are %v",
			branchName,
			availableBranches,
		)
	}

	return branchNode, nil
}

// Run implements the Node interface for Branch.
// It executes the main node first, then conditionally executes a branch based on the resolver.
//
// Execution logic:
//  1. Run the main node with the provided input
//  2. If branches are empty or resolver is nil, return the main node's output
//  3. Otherwise, resolve which branch to take based on input and output
//  4. Execute the selected branch node with the main node's output
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Input value for the main node
//
// Returns:
//   - The output from the executed branch (or main node if no branching occurs)
//   - An error if the main node, branch resolution, or branch execution fails
func (b *Branch) Run(ctx context.Context, input any) (any, error) {
	// Step 1: Execute the main node
	output, err := b.node.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	// Step 2: Check if branching is configured
	if len(b.branches) == 0 || b.branchResolver == nil {
		return output, nil
	}

	// Step 3: Resolve which branch to execute
	branchNode, err := b.resolveBranch(ctx, input, output)
	if err != nil {
		return nil, err
	}

	// Step 4: Execute the selected branch
	return branchNode.Run(ctx, output)
}
