// Package flow provides a composable pipeline framework for building data processing flows.
// It offers various node types that can be connected to create complex processing workflows.
package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Node is the interface that all flow nodes must implement.
// It defines the basic operations for nodes in a processing pipeline.
type Node interface {
	// SetNext connects this node to the next node in the pipeline.
	SetNext(next Node)

	// Run executes the node's functionality with the provided input and context.
	// It returns the processed output and any error that occurred during processing.
	Run(ctx context.Context, input any) (any, error)
}

var _ Node = (*OrderNode)(nil)

// OrderNode is the base implementation of the Node interface.
type OrderNode struct {
	next        Node
	executeFunc func(context.Context, any) (any, error)
}

// contextCheck checks if the context has been canceled.
func (n *OrderNode) contextCheck(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// execute runs the node's execution function and passes the result to the next node.
func (n *OrderNode) execute(ctx context.Context, input any) (any, error) {
	if n.executeFunc == nil {
		return nil, errors.New("execute function is required")
	}
	err := n.contextCheck(ctx)
	if err != nil {
		return nil, err
	}
	output, err := n.executeFunc(ctx, input)
	if err != nil {
		return nil, err
	}
	if n.next == nil {
		return output, nil
	}
	return n.next.Run(ctx, output)
}

// run wraps execute with additional error handling.
func (n *OrderNode) run(ctx context.Context, input any) (any, error) {
	output, err := n.execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("execute failed: %w", err)
	}
	return output, nil
}

// SetNext implements the Node interface by setting the next node in the pipeline.
func (n *OrderNode) SetNext(next Node) {
	if next != nil {
		n.next = next
	}
}

// Run implements the Node interface by executing the node's functionality.
func (n *OrderNode) Run(ctx context.Context, input any) (any, error) {
	return n.run(ctx, input)
}

// BranchNode is a node that can selectively route processing to different branches
// based on the output of a branch selection function.
type BranchNode struct {
	OrderNode
	branchFunc func(context.Context, any, any) (string, error)
	branches   map[string]Node
}

// selectBranch determines which branch to follow based on input and output.
func (b *BranchNode) selectBranch(ctx context.Context, i any, o any) (string, error) {
	if b.branchFunc == nil {
		return "", nil
	}
	err := b.contextCheck(ctx)
	if err != nil {
		return "", err
	}
	return b.branchFunc(ctx, i, o)
}

// Run implements the Node interface with branching behavior.
// It executes the node and then routes the output to the selected branch.
func (b *BranchNode) Run(ctx context.Context, input any) (any, error) {
	output, err := b.run(ctx, input)
	if err != nil {
		return nil, err
	}
	if b.branches == nil {
		return output, nil
	}
	branch, err := b.selectBranch(ctx, input, output)
	if err != nil {
		return nil, err
	}
	branchNode, ok := b.branches[branch]
	if !ok {
		return nil, fmt.Errorf("branch %s not found", branch)
	}
	return branchNode.Run(ctx, output)
}

// AddBranch adds a new branch node with the given action/key.
// Returns the branch node for chaining.
func (b *BranchNode) AddBranch(action string, node Node) Node {
	if b.branches == nil {
		b.branches = make(map[string]Node)
	}
	b.branches[action] = node
	return b
}

// LoopNode is a node that can repeatedly execute until a break condition is met.
type LoopNode struct {
	OrderNode
	breakFunc func(context.Context, int, any, any) (bool, error)
}

// shouldBreak determines if the looping should stop based on the current iteration.
func (l *LoopNode) shouldBreak(ctx context.Context, times int, input any, output any) (bool, error) {
	if l.breakFunc == nil {
		return true, nil
	}

	err := l.contextCheck(ctx)
	if err != nil {
		return false, err
	}
	return l.breakFunc(ctx, times, input, output)
}

// runWithTimes executes the node recursively with a counter until the break condition is met.
func (l *LoopNode) runWithTimes(ctx context.Context, times int, input any) (any, error) {
	output, err := l.run(ctx, input)
	if err != nil {
		return nil, err
	}

	shouldBreak, err := l.shouldBreak(ctx, times, input, output)
	if err != nil {
		return nil, err
	}
	if shouldBreak {
		return output, nil
	}
	return l.runWithTimes(ctx, times+1, input)
}

// Run implements the Node interface with looping behavior.
func (l *LoopNode) Run(ctx context.Context, input any) (any, error) {
	return l.runWithTimes(ctx, 0, input)
}

// BatchNode is a node that processes input in batches or chunks.
type BatchNode struct {
	OrderNode
	splitFunc   func(context.Context, any) ([]any, error)
	combineFunc func(context.Context, []any) (any, error)
}

// split divides the input into multiple chunks for processing.
func (b *BatchNode) split(ctx context.Context, input any) ([]any, error) {
	chunks, ok := input.([]any)
	if ok {
		return chunks, nil
	}
	if b.splitFunc == nil {
		return []any{input}, nil
	}
	err := b.contextCheck(ctx)
	if err != nil {
		return nil, err
	}
	return b.splitFunc(ctx, input)
}

// combine merges the processed outputs back into a single result.
func (b *BatchNode) combine(ctx context.Context, inputs []any) (any, error) {
	if b.combineFunc == nil {
		return inputs, nil
	}
	err := b.contextCheck(ctx)
	if err != nil {
		return nil, err
	}
	return b.combineFunc(ctx, inputs)
}

// Run implements the Node interface with batch processing behavior.
// It splits input, processes each chunk, and combines the results.
func (b *BatchNode) Run(ctx context.Context, input any) (any, error) {
	chunks, err := b.split(ctx, input)
	if err != nil {
		return nil, err
	}
	var (
		results []any
		errs    []error
	)

	for _, chunk := range chunks {
		output, err1 := b.run(ctx, chunk)
		if err1 != nil {
			errs = append(errs, err1)
			continue
		}
		results = append(results, output)
	}
	if len(results) == 0 {
		return nil, errors.Join(errs...)
	}
	return b.combine(ctx, results)
}

// ParallelNode is a node that processes input chunks in parallel.
type ParallelNode struct {
	BatchNode
}

// Run implements the Node interface with parallel processing behavior.
// It processes each chunk concurrently using goroutines.
func (p *ParallelNode) Run(ctx context.Context, input any) (any, error) {
	chunks, err := p.split(ctx, input)
	if err != nil {
		return nil, err
	}
	var (
		results []any
		errs    []error
		wg      sync.WaitGroup
		mu      sync.Mutex
	)
	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk any) {
			defer wg.Done()

			output, err1 := p.run(ctx, chunk)
			mu.Lock()
			if err1 != nil {
				errs = append(errs, err1)
			} else {
				results = append(results, output)
			}
			mu.Unlock()
		}(chunk)
	}
	wg.Wait()

	if len(results) == 0 {
		return nil, errors.Join(errs...)
	}
	return p.combine(ctx, results)
}

// AsyncNode is a node that executes its processing asynchronously.
type AsyncNode struct {
	OrderNode
}

// Run implements the Node interface with asynchronous behavior.
// It returns a channel that will receive the result when processing completes.
func (a *AsyncNode) Run(ctx context.Context, input any) (any, error) {
	channel := make(chan any, 1)
	go func() {
		defer close(channel)
		output, err := a.run(ctx, input)
		if err != nil {
			channel <- err
			return
		}
		channel <- output
	}()
	return channel, nil
}
