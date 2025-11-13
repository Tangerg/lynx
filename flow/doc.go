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

For complex workflows, use the Builder API with closure-based configuration
for better readability and scoped setup:

	flow, err := NewBuilder().
	    Then(validateNode).
	    Then(processNode).
	    Then(formatNode).
	    Build()

# Control Flow Patterns

Loop: Execute a node repeatedly until a termination condition is met.

Direct construction:

	loop, err := NewLoop(&LoopConfig[int, int]{
	    Node: incrementNode,
	    MaxIterations: 10,
	    Terminator: func(ctx context.Context, iteration int, input, output int) (bool, error) {
	        return output < 10, nil // Continue while output < 10
	    },
	})

Using Builder with closure:

	flow, err := NewBuilder().
	    Loop(func(loop *LoopBuilder) {
	        loop.WithNode(incrementNode).
	            WithMaxIterations(10).
	            WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
	                return output.(int) < 10, nil
	            })
	    }).
	    Build()

Branch: Execute different paths based on a condition.

Direct construction:

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

Using Builder with closure:

	flow, err := NewBuilder().
	    Branch(func(branch *BranchBuilder) {
	        branch.WithNode(decisionNode).
	            WithBranch("success", successNode).
	            WithBranch("failure", failureNode).
	            WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
	                if isValid(output) {
	                    return "success", nil
	                }
	                return "failure", nil
	            })
	    }).
	    Build()

# Parallel Processing

Parallel: Execute multiple nodes concurrently and aggregate results.

Direct construction:

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

Using Builder with closure:

	flow, err := NewBuilder().
	    Parallel(func(parallel *ParallelBuilder) {
	        parallel.WithNodes(node1, node2, node3).
	            WithWaitCount(2).
	            WithRequiredSuccesses(1).
	            WithContinueOnError().
	            WithAggregator(func(ctx context.Context, results []any) (any, error) {
	                combined := make([]string, len(results))
	                for i, r := range results {
	                    if r != nil {
	                        combined[i] = r.(string)
	                    }
	                }
	                return combined, nil
	            })
	    }).
	    Build()

Parallel execution strategies:

	// Wait for all nodes (default)
	.Parallel(func(p *ParallelBuilder) {
	    p.WithWaitAll().
	        WithNodes(nodes...)
	})

	// Wait for first completion (race mode)
	.Parallel(func(p *ParallelBuilder) {
	    p.WithWaitAny().
	        WithNodes(nodes...).
	        WithCancelRemaining()
	})

	// Wait for specific number with cancellation
	.Parallel(func(p *ParallelBuilder) {
	    p.WithWaitCount(3).
	        WithCancelRemaining().
	        WithNodes(nodes...)
	})

	// Fault-tolerant: N out of K must succeed
	.Parallel(func(p *ParallelBuilder) {
	    p.WithNodes(node1, node2, node3, node4, node5).
	        WithWaitCount(4).
	        WithRequiredSuccesses(3).
	        WithContinueOnError()
	})

Batch: Split input into segments, process in parallel, and aggregate results.

Direct construction:

	batch, err := NewBatch(&BatchConfig[[]int, int, int, int]{
	    Node: squareNode,
	    Segmenter: func(ctx context.Context, input []int) ([]int, error) {
	        segments := make([]int, len(input))
	        for i, v := range input {
	            segments[i] = v
	        }
	        return segments, nil
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

Using Builder with closure:

	flow, err := NewBuilder().
	    Batch(func(batch *BatchBuilder) {
	        batch.WithNode(processorNode).
	            WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
	                arr := input.([]int)
	                segments := make([]any, len(arr))
	                for i, v := range arr {
	                    segments[i] = v
	                }
	                return segments, nil
	            }).
	            WithAggregator(func(ctx context.Context, results []any) (any, error) {
	                sum := 0
	                for _, r := range results {
	                    if r != nil {
	                        sum += r.(int)
	                    }
	                }
	                return sum, nil
	            }).
	            WithConcurrencyLimit(10).
	            WithContinueOnError()
	    }).
	    Build()

# Asynchronous Execution

Async: Execute a node asynchronously and return a Future for the result.

Direct construction:

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

Using Builder with closure:

	flow, err := NewBuilder().
	    Then(preprocessNode).
	    Async(func(async *AsyncBuilder) {
	        async.WithNode(slowNode).
	            WithPool(threadPool)
	    }).
	    Then(postprocessNode). // Receives Future as input
	    Build()

# Complex Workflows

Combine multiple patterns for sophisticated pipelines using closure-based configuration:

	flow, err := NewBuilder().
	    // Initial validation
	    Then(validateInputNode).

	    // Parallel API calls
	    Parallel(func(parallel *ParallelBuilder) {
	        parallel.WithNodes(fetchUserNode, fetchOrdersNode, fetchInventoryNode).
	            WithWaitAll().
	            WithContinueOnError().
	            WithAggregator(combineDataNode)
	    }).

	    // Conditional processing
	    Branch(func(branch *BranchBuilder) {
	        branch.WithNode(checkStatusNode).
	            WithBranch("approved", approvalFlowNode).
	            WithBranch("pending", reviewFlowNode).
	            WithBranch("rejected", rejectionFlowNode).
	            WithBranchResolver(statusResolver)
	    }).

	    // Batch processing
	    Batch(func(batch *BatchBuilder) {
	        batch.WithNode(processItemNode).
	            WithSegmenter(splitItemsFunc).
	            WithAggregator(mergeResultsFunc).
	            WithConcurrencyLimit(20)
	    }).

	    // Async notification
	    Async(func(async *AsyncBuilder) {
	        async.WithNode(notifyNode).
	            WithPool(notificationPool)
	    }).

	    // Retry loop for failures
	    Loop(func(loop *LoopBuilder) {
	        loop.WithNode(retryNode).
	            WithMaxIterations(3).
	            WithTerminator(maxRetriesTerminator)
	    }).

	    // Final result
	    Then(formatOutputNode).
	    Build()

# Builder API Design

The Builder uses a closure-based configuration pattern that provides several benefits:

 1. Scoped Configuration: Each builder type (Loop, Branch, etc.) is configured
    within its own closure, making the configuration scope clear and preventing
    accidental cross-configuration.

 2. Automatic Building: Builders are automatically constructed and added to the
    flow when the closure completes, eliminating the need for manual Then() calls.

3. Type Safety: Configuration errors are caught at build time rather than runtime.

4. Readability: The nested structure mirrors the logical flow structure.

Example showing the pattern:

	flow, err := NewBuilder().
	    Then(node1).
	    Loop(func(loop *LoopBuilder) {
	        // Everything in this closure configures the loop
	        loop.WithNode(loopNode).
	            WithMaxIterations(5).
	            WithTerminator(terminator)
	        // Loop is automatically built and added when closure exits
	    }).
	    Then(node2). // Executes after the loop
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

Configure error handling behavior using closures:

	// Fail fast (default): stop on first error
	.Parallel(func(p *ParallelBuilder) {
	    p.WithNodes(node1, node2, node3).
	        WithAggregator(agg)
	})

	// Continue on error: collect all errors
	.Parallel(func(p *ParallelBuilder) {
	    p.WithNodes(node1, node2, node3).
	        WithContinueOnError().
	        WithRequiredSuccesses(2) // At least 2 must succeed
	})

	// Batch with error tolerance
	.Batch(func(b *BatchBuilder) {
	    b.WithNode(processorNode).
	        WithContinueOnError() // Skip failed segments
	})

# Best Practices

1. Use Processor for simple transformations:

	uppercase := Processor[string, string](func(ctxcontext.Context,inputstring) (string, error) {
	    return strings.ToUpper(input), nil
	})

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

3. Use closure-based Builder API for readability in complex workflows:

	flow, err := NewBuilder().
	    Then(step1).
	    Parallel(func(p *ParallelBuilder) {
	        p.WithNodes(step2a, step2b).
	            WithAggregator(merge)
	    }).
	    Then(step3).
	    Build()

4. Keep closures focused and concise:

	// Good: focused configuration
	.Loop(func(loop *LoopBuilder) {
	    loop.WithNode(retryNode).
	        WithMaxIterations(3)
	})

	// Avoid: complex logic in closures
	.Loop(func(loop *LoopBuilder) {
	    // Don't put business logic here
	    node := createComplexNode()
	    terminator := buildTerminator()
	    loop.WithNode(node).WithTerminator(terminator)
	})

5. Leverage context for cancellation and timeouts:

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := flow.Run(ctx, input)

6. Configure concurrency limits to control resource usage:

	.Batch(func(b *BatchBuilder) {
	    b.WithConcurrencyLimit(runtime.NumCPU())
	})

	.Parallel(func(p *ParallelBuilder) {
	    p.WithNodes(nodes...).
	        WithWaitCount(runtime.NumCPU())
	})

7. Use type-safe methods when possible:

	// Preferred: type-safe
	future, err := async.RunType(ctx, input)
	result, err := future.Get()

	// Avoid: requires type assertion
	anyFuture, err := async.Run(ctx, input)
	future := anyFuture.(sync.Future[string])

8. Build flows once and reuse them:

	// Build once
	flow, err := NewBuilder().
	    Then(node1).
	    Then(node2).
	    Build()
	if err != nil {
	    log.Fatal(err)
	}

	// Reuse many times
	for _, input := range inputs {
	    result, err := flow.Run(ctx, input)
	    // Process result
	}

# Performance Considerations

- Parallel and Batch nodes create goroutines; use concurrency limits to control overhead
- Async nodes return immediately but consume pool resources until completion
- Loop nodes run synchronously; consider async execution for long-running loops
- Branch nodes evaluate the main node before branching; optimize the decision node
- Use context cancellation to stop expensive operations early
- Builder has minimal overhead; closures are executed only during Build(), not during Run()

# Thread Safety

All nodes are safe for concurrent use once constructed. However, be cautious with
shared state in Processor functions and custom node implementations.

Safe:

	node := Processor[int, int](func(ctxcontext.Context,inputint) (int, error) {
	    return input + 1, nil // No shared state
	})

Unsafe without synchronization:

	counter := 0
	node := Processor[int, int](func(ctxcontext.Context,inputint) (int, error) {
	    counter++ // Data race!
	    return counter, nil
	})

Safe with synchronization:

	var mu sync.Mutex
	counter := 0
	node := Processor[int, int](func(ctxcontext.Context,inputint) (int, error) {
	    mu.Lock()
	    defer mu.Unlock()
	    counter++
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

Test complete flows with closures:

	func TestWorkflow(t *testing.T) {
	    flow, err := NewBuilder().
	        Then(node1).
	        Parallel(func(p *ParallelBuilder) {
	            p.WithNodes(node2, node3).
	                WithAggregator(testAggregator)
	        }).
	        Build()

	    require.NoError(t, err)

	    output, err := flow.Run(context.Background(), testInput)
	    assert.NoError(t, err)
	    assert.Equal(t, expectedOutput, output)
	}

Mock nodes for testing:

	type MockNode struct {
	    RunFunc func(context.Context, any) (any, error)
	}

	func (m *MockNode) Run(ctx context.Context, input any) (any, error) {
	    return m.RunFunc(ctx, input)
	}

	func TestWithMock(t *testing.T) {
	    mock := &MockNode{
	        RunFunc: func(ctx context.Context, input any) (any, error) {
	            return "mocked", nil
	        },
	    }

	    flow, err := NewBuilder().
	        Then(mock).
	        Build()

	    // Test flow with mock
	}
*/
package flow
