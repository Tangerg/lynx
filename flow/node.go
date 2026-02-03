package flow

import (
	"context"
	"errors"
	"fmt"
)

// Node represents a processing unit in the workflow that can transform input to output.
// The generic parameters I and O define the input and output types for the node.
type Node[I any, O any] interface {
	// Run executes the node's processing logic with the provided context and input.
	// Returns the processed output and any error that occurred during processing.
	Run(ctx context.Context, input I) (O, error)
}

// Result encapsulates the outcome of an operation, containing either a value or an error.
type Result[V any] struct {
	Value V
	Error error
}

// Pipe creates a sequential pipeline from multiple nodes with dynamic typing.
// All nodes must accept and return 'any' type, losing compile-time type safety.
// The output of each node becomes the input of the next node in the chain.
//
// Returns an error if no nodes are provided.
// For type-safe pipelines, use Pipe2 through Pipe10 instead.
func Pipe(nodes ...Node[any, any]) (Node[any, any], error) {
	if len(nodes) == 0 {
		return nil, errors.New("pipe requires at least one node")
	}

	if len(nodes) == 1 {
		return nodes[0], nil
	}

	// Convert nodes to processor functions
	processors := make([]func(context.Context, any) (any, error), len(nodes))
	for i, node := range nodes {
		processors[i] = node.Run
	}

	sequence, err := NewSequence(processors...)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe sequence: %w", err)
	}

	return sequence, nil
}

// Pipe2 creates a type-safe pipeline of two nodes.
// The output type of the first node must match the input type of the second node.
//
// Generic parameters:
//   - I: Input type of the first node
//   - M: Intermediate type (output of first node, input of second node)
//   - O: Output type of the second node
func Pipe2[I, M, O any](first Node[I, M], second Node[M, O]) Node[I, O] {
	return Func[I, O](func(ctx context.Context, input I) (O, error) {
		intermediate, err := first.Run(ctx, input)
		if err != nil {
			var zero O
			return zero, fmt.Errorf("pipe2: first node failed: %w", err)
		}

		output, err := second.Run(ctx, intermediate)
		if err != nil {
			var zero O
			return zero, fmt.Errorf("pipe2: second node failed: %w", err)
		}

		return output, nil
	})
}

// Pipe3 creates a type-safe pipeline of three nodes.
// Each node's output type must match the next node's input type.
//
// Generic parameters:
//   - I: Input type of the first node
//   - M1, M2: Intermediate types
//   - O: Output type of the final node
func Pipe3[I, M1, M2, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, O]) Node[I, O] {
	return Pipe2(Pipe2(n1, n2), n3)
}

// Pipe4 creates a type-safe pipeline of four nodes.
func Pipe4[I, M1, M2, M3, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, O]) Node[I, O] {
	return Pipe2(Pipe3(n1, n2, n3), n4)
}

// Pipe5 creates a type-safe pipeline of five nodes.
func Pipe5[I, M1, M2, M3, M4, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, O]) Node[I, O] {
	return Pipe2(Pipe4(n1, n2, n3, n4), n5)
}

// Pipe6 creates a type-safe pipeline of six nodes.
func Pipe6[I, M1, M2, M3, M4, M5, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, M5], n6 Node[M5, O]) Node[I, O] {
	return Pipe2(Pipe5(n1, n2, n3, n4, n5), n6)
}

// Pipe7 creates a type-safe pipeline of seven nodes.
func Pipe7[I, M1, M2, M3, M4, M5, M6, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, M5], n6 Node[M5, M6], n7 Node[M6, O]) Node[I, O] {
	return Pipe2(Pipe6(n1, n2, n3, n4, n5, n6), n7)
}

// Pipe8 creates a type-safe pipeline of eight nodes.
func Pipe8[I, M1, M2, M3, M4, M5, M6, M7, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, M5], n6 Node[M5, M6], n7 Node[M6, M7], n8 Node[M7, O]) Node[I, O] {
	return Pipe2(Pipe7(n1, n2, n3, n4, n5, n6, n7), n8)
}

// Pipe9 creates a type-safe pipeline of nine nodes.
func Pipe9[I, M1, M2, M3, M4, M5, M6, M7, M8, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, M5], n6 Node[M5, M6], n7 Node[M6, M7], n8 Node[M7, M8], n9 Node[M8, O]) Node[I, O] {
	return Pipe2(Pipe8(n1, n2, n3, n4, n5, n6, n7, n8), n9)
}

// Pipe10 creates a type-safe pipeline of ten nodes.
func Pipe10[I, M1, M2, M3, M4, M5, M6, M7, M8, M9, O any](n1 Node[I, M1], n2 Node[M1, M2], n3 Node[M2, M3], n4 Node[M3, M4], n5 Node[M4, M5], n6 Node[M5, M6], n7 Node[M6, M7], n8 Node[M7, M8], n9 Node[M8, M9], n10 Node[M9, O]) Node[I, O] {
	return Pipe2(Pipe9(n1, n2, n3, n4, n5, n6, n7, n8, n9), n10)
}
