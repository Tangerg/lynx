/*
Package flow provides a flexible and composable framework for building complex
data processing workflows with support for sequential, parallel, conditional,
and iterative execution patterns.

# Overview

The flow package enables you to construct sophisticated data processing pipelines
by composing simple, reusable nodes. Each node represents a discrete processing
step that transforms input data into output data. Nodes can be combined in various
ways to create complex workflows while maintaining clean, readable code.

# Core Concepts

Node: The fundamental building block that processes input and produces output.
Any type implementing the Node interface can be used in a workflow:

	type Node[I any, O any] interface {
	    Run(ctx context.Context, input I) (O, error)
	}

Processor: A function type that implements Node, providing a convenient way
to create nodes from simple functions:

	uppercase := Processor[string, string](func(ctxcontext.Context,inputstring) (string, error) {
	    return strings.ToUpper(input), nil
	})

# Basic Usage

The simplest workflow is a sequential chain of processing steps:

	// Create individual nodes
	validateNode := Processor[string, string](func(ctxcontext.Context,inputstring) (string, error) {
	    if input == "" {
	        return "", errors.New("input cannot be empty")
	    }
	    return input, nil
	})

	processNode := Processor[string, string](func(ctxcontext.Context,inputstring) (string, error) {
	    return strings.ToUpper(input), nil
	})

	// Combine into a flow
	flow, err := NewFlow(validateNode, processNode)
	if err != nil {
	    log.Fatal(err)
	}

	// Execute the flow
	result, err := flow.Run(ctx, "hello")
	// result = "HELLO"

# Flow Builder

For complex workflows, use the Builder API for better readability:

	flow, err := NewBuilder().
	    Then(validateNode).
	    Then(processNode).
	    Then(formatNode).
	    Build()

# Control Flow Patterns

Loop: Execute a node repeatedly until a termination condition is met.

	loop, err := NewLoop(&LoopConfig[int, int]{
	    Node: incrementNode,
	    Terminator: func(ctx context.Context, iteration int, input, output int) (bool, error) {
	        return output < 10, nil // Continue while output < 10
	    },
	})

Using Builder:

	flow, err := NewBuilder().
	    Loop().
	        WithNode(incrementNode).
	        WithTerminator(terminatorFunc).
	        Then().
	    Build()

Branch: Execute different paths based on a condition.

	branch, err := NewBranch(&BranchConfig{
	    Node: validatorNode,
	    BranchResolver: func(ctx context.Context, input, output any) (string, error) {
	        if isValid(output) {
	            return "success", nil
	        }
	        return "failure", nil
	    },
	    Branches: map[string]Node[any, any]{
	        "success": successNode,
	        "failure": retryNode,
	    },
	})

Using Builder:

	flow, err := NewBuilder().
	    Branch().
	        WithNode(decisionNode).
	        WithBranch("success", successNode).
	        WithBranch("failure", failureNode).
	        WithBranchResolver(resolver).
	        Then().
	    Build()

# Parallel Processing

Parallel: Execute multiple nodes concurrently and aggregate results.

	parallel, err := NewParallel(&ParallelConfig[string, []string]{
	    Nodes: []Node[string, any]{serviceA, serviceB, serviceC},
	    Aggregator: func(ctx context.Context, results []any) ([]string, error) {
	        combined := make([]string, len(results))
	        for i, r := range results {
	            combined[i] = r.(string)
	        }
	        return combined, nil
	    },
	    WaitCount: 2,           // Wait for first 2 completions
	    RequiredSuccesses: 1,   // At least 1 must succeed
	    ContinueOnError: true,  // Don't stop on first error
	})

Using Builder:

	flow, err := NewBuilder().
	    Parallel().
	        WithNodes(node1, node2, node3).
	        WithAggregator(aggregatorFunc).
	        WithWaitCount(2).
	        WithRequiredSuccesses(1).
	        WithContinueOnError().
	        Then().
	    Build()

Parallel execution strategies:

	// Wait for all nodes (default)
	.Parallel().WithWaitAll()...

	// Wait for first completion (race mode)
	.Parallel().WithWaitAny()...

	// Wait for specific number with cancellation
	.Parallel().
	    WithWaitCount(3).
	    WithCancelRemaining()...

	// Fault-tolerant: N out of K must succeed
	.Parallel().
	    WithNodes(node1, node2, node3, node4, node5).
	    WithWaitCount(4).
	    WithRequiredSuccesses(3).
	    WithContinueOnError()...

Batch: Split input into segments, process in parallel, and aggregate results.

	batch, err := NewBatch(&BatchConfig[[]int, int, int, int]{
	    Node: squareNode,
	    Segmenter: func(ctx context.Context, input []int) ([]int, error) {
	        return input, nil // Each element is a segment
	    },
	    Aggregator: func(ctx context.Context, results []int) (int, error) {
	        sum := 0
	        for _, r := range results {
	            sum += r
	        }
	        return sum, nil
	    },
	    ConcurrencyLimit: 10,
	    ContinueOnError: true,
	})

Using Builder:

	flow, err := NewBuilder().
	    Batch().
	        WithNode(processorNode).
	        WithSegmenter(segmenterFunc).
	        WithAggregator(aggregatorFunc).
	        WithConcurrencyLimit(10).
	        WithContinueOnError().
	        Then().
	    Build()

# Asynchronous Execution

Async: Execute a node asynchronously and return a Future for the result.

	async, err := NewAsync(&AsyncConfig[string, string]{
	    Node: slowNode,
	    Pool: threadPool,
	})

	// Returns immediately with a Future
	future, err := async.RunType(ctx, input)

	// Do other work...
	doOtherWork()

	// Get result when needed
	result, err := future.Get()

Using Builder:

	flow, err := NewBuilder().
	    Then(preprocessNode).
	    Async().
	        WithNode(slowNode).
	        WithPool(threadPool).
	        Then().
	    Then(postprocessNode). // Receives Future as input
	    Build()

# Complex Workflows

Combine multiple patterns for sophisticated pipelines:

	flow, err := NewBuilder().
	    // Initial validation
	    Then(validateInputNode).

	    // Parallel API calls
	    Parallel().
	        WithNodes(fetchUserNode, fetchOrdersNode, fetchInventoryNode).
	        WithAggregator(combineDataNode).
	        WithWaitAll().
	        WithContinueOnError().
	        Then().

	    // Conditional processing
	    Branch().
	        WithNode(checkStatusNode).
	        WithBranch("approved", approvalFlowNode).
	        WithBranch("pending", reviewFlowNode).
	        WithBranch("rejected", rejectionFlowNode).
	        WithBranchResolver(statusResolver).
	        Then().

	    // Batch processing
	    Batch().
	        WithNode(processItemNode).
	        WithSegmenter(splitItemsFunc).
	        WithAggregator(mergeResultsFunc).
	        WithConcurrencyLimit(20).
	        Then().

	    // Async notification
	    Async().
	        WithNode(notifyNode).
	        WithPool(notificationPool).
	        Then().

	    // Retry loop for failures
	    Loop().
	        WithNode(retryNode).
	        WithTerminator(maxRetriesTerminator).
	        Then().

	    // Final result
	    Then(formatOutputNode).
	    Build()

# Error Handling

All nodes support context-aware error handling:

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := flow.Run(ctx, input)
	if err != nil {
	    if errors.Is(err, context.DeadlineExceeded) {
	        // Handle timeout
	    } else if errors.Is(err, context.Canceled) {
	        // Handle cancellation
	    } else {
	        // Handle other errors
	    }
	}

Configure error handling behavior:

	// Fail fast (default): stop on first error
	.Parallel().
	    WithNodes(node1, node2, node3).
	    WithAggregator(agg)...

	// Continue on error: collect all errors
	.Parallel().
	    WithNodes(node1, node2, node3).
	    WithContinueOnError().
	    WithRequiredSuccesses(2)... // At least 2 must succeed

	// Batch with error tolerance
	.Batch().
	    WithNode(processorNode).
	    WithContinueOnError()... // Skip failed segments

# Best Practices

1. Use Processor for simple transformations:

	uppercase := Processor[string, string](strings.ToUpper)

2. Define reusable nodes as structs for complex logic:

	type ValidationNode struct {
	    validator Validator
	}

	func (n *ValidationNode) Run(ctx context.Context, input Data) (Data, error) {
	    if err := n.validator.Validate(input); err != nil {
	        return Data{}, err
	    }
	    return input, nil
	}

3. Use Builder API for readability in complex workflows:

	flow, err := NewBuilder().
	    Then(step1).
	    Parallel().
	        WithNodes(step2a, step2b).
	        WithAggregator(merge).
	        Then().
	    Then(step3).
	    Build()

4. Leverage context for cancellation and timeouts:

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := flow.Run(ctx, input)

5. Configure concurrency limits to control resource usage:

	.Batch().
	    WithConcurrencyLimit(runtime.NumCPU())...

	.Parallel().
	    WithNodes(nodes...).
	    WithWaitCount(runtime.NumCPU())...

6. Use type-safe methods when possible:

	// Preferred: type-safe
	future, err := async.RunType(ctx, input)
	result, err := future.Get()

	// Avoid: requires type assertion
	anyFuture, err := async.Run(ctx, input)
	future := anyFuture.(sync.Future[string])

# Performance Considerations

- Parallel and Batch nodes create goroutines; use concurrency limits to control overhead
- Async nodes return immediately but consume pool resources until completion
- Loop nodes run synchronously; consider async execution for long-running loops
- Branch nodes evaluate the main node before branching; optimize the decision node
- Use context cancellation to stop expensive operations early

# Thread Safety

All nodes are safe for concurrent use once constructed. However, be cautious with
shared state in Processor functions and custom node implementations.

Safe:

	counter := 0
	node := Processor[int, int](func(ctxcontext.Context,inputint) (int, error) {
	    return input + 1, nil // No shared state
	})

Unsafe without synchronization:

	counter := 0
	node := Processor[int, int](func(ctxcontext.Context,inputint) (int, error) {
	    counter++ // Data race!
	    return counter, nil
	})

# Testing

Nodes are easy to test in isolation:

	func TestMyNode(t *testing.T) {
	    node := &MyNode{config: testConfig}
	    output, err := node.Run(context.Background(), testInput)

	    assert.NoError(t, err)
	    assert.Equal(t, expectedOutput, output)
	}

Test complete flows:

	func TestWorkflow(t *testing.T) {
	    flow, err := NewBuilder().
	        Then(node1).
	        Then(node2).
	        Build()

	    require.NoError(t, err)

	    output, err := flow.Run(context.Background(), testInput)
	    assert.NoError(t, err)
	    assert.Equal(t, expectedOutput, output)
	}
*/
package flow
