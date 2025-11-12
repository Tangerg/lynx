package flow

import (
	"context"
	"errors"
)

// Flow represents a sequential workflow that executes a series of nodes in order.
// Each node's output becomes the input for the next node in the chain.
type Flow struct {
	nodes []Node[any, any]
}

// NewFlow creates a new Flow instance with the provided nodes.
// Returns an error if no nodes are provided.
//
// Example:
//
//	flow, err := NewFlow(node1, node2, node3)
func NewFlow(nodes ...Node[any, any]) (*Flow, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no nodes provided")
	}

	return &Flow{nodes: nodes}, nil
}

// Run executes all nodes in the flow sequentially with the given input.
// The output of each node becomes the input for the next node.
// Execution stops immediately if any node returns an error.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - input: Initial input value for the first node
//
// Returns:
//   - The final output after all nodes have been executed
//   - An error if any node fails during execution
func (f *Flow) Run(ctx context.Context, input any) (any, error) {
	current := input

	for _, node := range f.nodes {
		result, err := node.Run(ctx, current)
		if err != nil {
			return nil, err
		}

		current = result
	}

	return current, nil
}
