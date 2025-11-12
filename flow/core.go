package flow

import (
	"context"
)

// Node represents a processing unit in the workflow that can transform input to output.
// The generic parameters I and O define the input and output types for the node.
type Node[I any, O any] interface {
	// Run executes the node's processing logic with the provided context and input.
	// Returns the processed output and any error that occurred during processing.
	Run(ctx context.Context, input I) (O, error)
}

// Middleware is a higher-order function that can modify or enhance the behavior of a Node.
// It takes a Node as input and returns a potentially modified Node with the same input/output types.
type Middleware[I any, O any] func(node Node[I, O]) Node[I, O]

// Join combines multiple nodes into a single flow.
// The nodes are executed in sequence, with each node's output becoming the next node's input.
// Returns the combined flow or an error if no nodes are provided.
func Join(nodes ...Node[any, any]) (Node[any, any], error) {
	return NewFlow(nodes...)
}
