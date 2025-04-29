package flow

import (
	"errors"
)

// Chain combines multiple nodes into a single sequential processing pipeline.
//
// This utility function creates a flow where each node's output is passed to the next node.
// It simplifies the creation of linear processing chains without needing to use the builder API.
//
// Parameters:
//   - nodes: A variadic list of Node[any, any] instances to chain together sequentially
//
// Returns:
//   - A single Node[any, any] representing the entire chain
//   - An error if no nodes were provided or if the flow couldn't be built
//
// Example:
//
//	chain, err := flow.Chain(
//	    preprocessor,
//	    transformer,
//	    validator,
//	    outputFormatter,
//	)
//	if err != nil {
//	    // Handle error
//	}
//	result, err := chain.Run(ctx, input)
func Chain(nodes ...Node[any, any]) (Node[any, any], error) {
	if len(nodes) == 0 {
		return nil, errors.New("at least one node is required")
	}
	flow := NewFlow()
	for _, node := range nodes {
		flow.Then(node)
	}
	return flow.Build()
}

// OfNode creates a Flow from an existing Node.
//
// This utility function provides a convenient way to start building a flow
// pipeline using an existing node as the starting point. This is useful for
// extending the functionality of pre-configured nodes or for composing
// complex flows from simpler components.
//
// Parameters:
//   - node: The existing Node[any, any] to be used as the starting point
//
// Returns:
//   - A Flow instance with the provided node as its starting point
//
// Example:
//
//	existingNode := getExistingNode()
//	flow.OfNode(existingProcessor).
//	    Next().
//	    Sequence().
//	    WithProcessor(additionalProcessing).
//	    End().
//	    Build()
func OfNode(node Node[any, any]) *Flow {
	return NewFlow().Then(node)
}

// OfProcessor creates a Flow from a processing function.
//
// This utility function simplifies the common case of creating a flow
// that starts with a single processing function. It automatically wraps
// the provided processor in a SequenceNode and returns a Flow ready for
// further configuration.
//
// Parameters:
//   - processor: A function that implements the Processor[any, any] interface
//
// Returns:
//   - A Flow instance with a SequenceNode containing the provided processor
//
// Example:
//
//	flow.OfProcessor(func(ctx context.Context, input any) (any, error) {
//	    return fmt.Sprintf("Processed: %v", input), nil
//	}).
//	Next().
//	Branch().
//	WithRouteSelector(routeSelector).
//	AddBranch("route1", handler1).
//	End().
//	Build()
func OfProcessor(processor Processor[any, any]) *Flow {
	return NewFlow().Sequence().WithProcessor(processor).End()
}
