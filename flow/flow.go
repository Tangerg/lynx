package flow

import (
	"context"
	"errors"
)

// Flow represents a sequence of processing nodes that are executed in order.
// It implements the Node interface and can be used as a building block for complex workflows.
type Flow struct {
	// nodes contains the ordered sequence of processing nodes that make up the flow
	nodes []Node[any, any]
}

// NewFlow creates a new empty flow with no processing nodes.
// The flow can be built by adding nodes using the Then or Step methods.
func NewFlow() *Flow {
	return &Flow{}
}

// validate ensures the flow is properly constructed with at least one node.
// Returns an error if the flow is empty.
func (f *Flow) validate() error {
	if len(f.nodes) == 0 {
		return errors.New("flow must contain at least one node: current flow is empty")
	}
	return nil
}

// Run executes the flow by passing the input through each node in sequence.
// The output of each node becomes the input to the next node.
// Returns the final output and any error encountered during execution.
func (f *Flow) Run(ctx context.Context, input any) (any, error) {
	err := f.validate()
	if err != nil {
		return nil, err
	}
	var output = input
	for _, node := range f.nodes {
		output, err = node.Run(ctx, output)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}

// Then adds a Node to the end of the flow.
// Returns the flow for chaining. Ignores nil nodes.
func (f *Flow) Then(node Node[any, any]) *Flow {
	if node != nil {
		f.nodes = append(f.nodes, node)
	}
	return f
}

// Step adds a Processor function as a node to the end of the flow.
// It's a convenience method that wraps the processor in a Node interface.
// Returns the flow for chaining.
func (f *Flow) Step(processor Processor[any, any]) *Flow {
	f.Then(processor)
	return f
}

// Branch creates a new BranchBuilder to add conditional branching to the flow.
// This allows the flow to take different paths based on the result of a processor.
func (f *Flow) Branch() *BranchBuilder { return &BranchBuilder{flow: f, branch: &Branch[string]{}} }

// Loop creates a new LoopBuilder to add iterative processing to the flow.
// This allows repeated execution of a processor until a condition is met.
func (f *Flow) Loop() *LoopBuilder { return &LoopBuilder{flow: f, loop: &Loop[any, any]{}} }

// Batch creates a new BatchBuilder to add batch processing to the flow.
// This allows processing collections of items with optional concurrency.
func (f *Flow) Batch() *BatchBuilder {
	return &BatchBuilder{flow: f, batch: &Batch[any, any, any, any]{}}
}

// Async creates a new AsyncBuilder to add asynchronous processing to the flow.
// This allows processing to continue without waiting for completion.
func (f *Flow) Async() *AsyncBuilder { return &AsyncBuilder{flow: f, async: &Async[any, any]{}} }

// Parallel creates a new ParallelBuilder to add parallel processing to the flow.
// This allows multiple processors to execute concurrently and combine their results.
func (f *Flow) Parallel() *ParallelBuilder {
	return &ParallelBuilder{flow: f, parallel: &Parallel[any, any]{}}
}
