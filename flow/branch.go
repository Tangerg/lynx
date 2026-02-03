package flow

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// BranchConfig defines the configuration for a branch node that routes execution
// to different processing paths based on runtime conditions.
type BranchConfig[I, O any] struct {
	// Branches maps branch names to their corresponding processor functions.
	// Each branch must accept the same input type and produce the same output type.
	Branches map[string]func(context.Context, I) (O, error)

	// BranchResolver determines which branch to execute based on the input.
	// It returns the name of the branch to be executed.
	// If nil and only one branch exists, that branch will be used by default.
	BranchResolver func(context.Context, I) string
}

// validate checks if the branch configuration is valid and applies defaults.
func (cfg *BranchConfig[I, O]) validate() error {
	if cfg == nil {
		return errors.New("branch config cannot be nil")
	}

	if len(cfg.Branches) == 0 {
		return errors.New("at least one branch is required")
	}

	// Optimization: if only one branch exists and not provide branch resolver, use it as default
	if len(cfg.Branches) == 1 && cfg.BranchResolver == nil {
		var defaultBranch string
		for defaultBranch = range cfg.Branches {
			break
		}
		cfg.BranchResolver = func(context.Context, I) string {
			return defaultBranch
		}
	}

	if cfg.BranchResolver == nil {
		return errors.New("branch resolver cannot be nil for multiple branches")
	}

	return nil
}

var _ Node[any, any] = (*Branch[any, any])(nil)

// Branch represents a node that conditionally routes execution to different branches
// based on input characteristics.
type Branch[I, O any] struct {
	branches       map[string]func(context.Context, I) (O, error)
	branchResolver func(context.Context, I) string
}

// NewBranch creates a new branch node with the provided configuration.
// Returns an error if the configuration is invalid.
func NewBranch[I, O any](cfg BranchConfig[I, O]) (*Branch[I, O], error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid branch config: %w", err)
	}

	return &Branch[I, O]{
		branches:       maps.Clone(cfg.Branches),
		branchResolver: cfg.BranchResolver,
	}, nil
}

// resolveBranch determines and retrieves the branch to execute based on the input.
// Returns an error if the resolved branch name does not exist.
func (b *Branch[I, O]) resolveBranch(ctx context.Context, input I) (func(context.Context, I) (O, error), error) {
	branchName := b.branchResolver(ctx, input)

	branch, exists := b.branches[branchName]
	if !exists {
		availableBranches := slices.Collect(maps.Keys(b.branches))
		return nil, fmt.Errorf(
			"branch '%s' not found: available branches are %v",
			branchName,
			availableBranches,
		)
	}

	return branch, nil
}

// Run executes the appropriate branch based on the input.
// The branch is selected by the BranchResolver, then executed with the provided input.
func (b *Branch[I, O]) Run(ctx context.Context, input I) (O, error) {
	branch, err := b.resolveBranch(ctx, input)
	if err != nil {
		var zero O
		return zero, fmt.Errorf("failed to resolve branch: %w", err)
	}

	result, err := branch(ctx, input)
	if err != nil {
		var zero O
		return zero, fmt.Errorf("branch execution failed: %w", err)
	}

	return result, nil
}
