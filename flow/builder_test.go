package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/pkg/sync"
)

// TestLoopBuilder tests the LoopBuilder functionality with closure-based configuration
func TestLoopBuilder(t *testing.T) {
	t.Run("builds loop with node only", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node)
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Without terminator, should execute exactly once
		if counter != 1 {
			t.Errorf("expected 1 execution, got %d", counter)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("builds loop with terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration >= 2, nil // Stop after 3 iterations (0, 1, 2)
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 3 {
			t.Errorf("expected 3 executions, got %d", counter)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("builds loop with max iterations only", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + counter, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithMaxIterations(5)
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)

		if counter != 5 {
			t.Errorf("expected 5 executions, got %d", counter)
		}

		if result.(int) != 5 {
			t.Errorf("expected 5, got %d", result.(int))
		}
	})

	t.Run("builds loop with both max iterations and terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithMaxIterations(10).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration >= 2, nil // Would stop at iteration 2
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Terminator triggers first
		if counter != 3 {
			t.Errorf("expected 3 executions, got %d", counter)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("max iterations reached before terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithMaxIterations(3).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return false, nil // Never terminate
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)

		if counter != 3 {
			t.Errorf("expected 3 executions, got %d", counter)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("method chaining within closure", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				result := loop.
					WithNode(node).
					WithMaxIterations(5).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration >= 2, nil
					})

				// Verify method chaining returns same builder instance
				if result != loop {
					t.Error("expected method chaining to return same builder instance")
				}
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("fails with invalid config - nil node", func(t *testing.T) {
		_, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				// Don't set node
			}).
			Build()

		if err == nil {
			t.Error("expected error for invalid loop config")
		}

		if !strings.Contains(err.Error(), "loop node cannot be nil") {
			t.Errorf("expected 'loop node cannot be nil' error, got %v", err)
		}
	})

	t.Run("multiple loops in sequence", func(t *testing.T) {
		counter1 := 0
		counter2 := 0

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter1++
			return input.(int) + 1, nil
		})

		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter2++
			return input.(int) * 2, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node1).
					WithMaxIterations(3).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration >= 1, nil
					})
			}).
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node2).
					WithMaxIterations(2).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration >= 1, nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter1 != 2 {
			t.Errorf("expected first loop to execute 2 times, got %d", counter1)
		}

		if counter2 != 2 {
			t.Errorf("expected second loop to execute 2 times, got %d", counter2)
		}

		// First loop: 0 + 1 = 1
		// Second loop: 1 * 2 = 2
		if result.(int) != 2 {
			t.Errorf("expected 2, got %d", result.(int))
		}
	})

	t.Run("ignores nil node", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(nil). // Should be ignored
							WithNode(node) // This should be used
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 1 {
			t.Errorf("expected 1 execution, got %d", counter)
		}
	})

	t.Run("ignores zero max iterations", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithMaxIterations(0) // Should be ignored
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		// Should still work, just executes once by default
		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores negative max iterations", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithMaxIterations(-5) // Should be ignored
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores nil terminator", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).
					WithTerminator(nil) // Should be ignored
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestBranchBuilder tests the BranchBuilder functionality with closure-based configuration
func TestBranchBuilder(t *testing.T) {
	t.Run("builds branch with all components", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "success", nil
		})
		successNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "success result", nil
		})
		failureNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "failure result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(decisionNode).
					WithBranch("success", successNode).
					WithBranch("failure", failureNode).
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "success result" {
			t.Errorf("expected 'success result', got %s", result.(string))
		}
	})

	t.Run("builds branch with WithBranches", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1", nil
		})
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result1", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result2", nil
		})

		branches := map[string]Node[any, any]{
			"branch1": node1,
			"branch2": node2,
		}

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(decisionNode).
					WithBranches(branches).
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "result1" {
			t.Errorf("expected 'result1', got %s", result.(string))
		}
	})

	t.Run("ignores nil node", func(t *testing.T) {
		validNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1", nil
		})
		branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(nil). // Should be ignored
							WithNode(validNode). // This should be used
							WithBranch("branch1", branchNode).
							WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "result" {
			t.Errorf("expected 'result', got %s", result.(string))
		}
	})

	t.Run("ignores nil branch node", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "valid", nil
		})
		validBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(decisionNode).
					WithBranch("nil", nil). // Should be ignored
					WithBranch("valid", validBranch).
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return "valid", nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "result" {
			t.Errorf("expected 'result', got %s", result.(string))
		}
	})

	t.Run("ignores nil branches map", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1", nil
		})
		existingBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(decisionNode).
					WithBranch("branch1", existingBranch).
					WithBranches(nil). // Should be ignored
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "result" {
			t.Errorf("expected 'result', got %s", result.(string))
		}
	})

	t.Run("ignores nil resolver", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1", nil
		})
		branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(decisionNode).
					WithBranch("branch1", branchNode).
					WithBranchResolver(nil). // Should be ignored
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "result" {
			t.Errorf("expected 'result', got %s", result.(string))
		}
	})

	t.Run("method chaining within closure", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1", nil
		})
		branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result", nil
		})

		flow, err := NewBuilder().
			Branch(func(branch *BranchBuilder) {
				result := branch.
					WithNode(decisionNode).
					WithBranch("branch1", branchNode).
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						return output.(string), nil
					})

				// Verify method chaining returns same builder instance
				if result != branch {
					t.Error("expected method chaining to return same builder instance")
				}
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestBatchBuilder tests the BatchBuilder functionality with closure-based configuration
func TestBatchBuilder(t *testing.T) {
	t.Run("builds batch with all components", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(node).
					WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
						arr := input.([]int)
						result := make([]any, len(arr))
						for i, v := range arr {
							result[i] = v
						}
						return result, nil
					}).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						sum := 0
						for _, r := range results {
							sum += r.(int)
						}
						return sum, nil
					}).
					WithConcurrencyLimit(2).
					WithContinueOnError()
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), []int{1, 2, 3, 4, 5})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 30 {
			t.Errorf("expected 30, got %d", result.(int))
		}
	})

	t.Run("ignores zero concurrency limit", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(node).
					WithConcurrencyLimit(0). // Should be ignored
					WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
						return []any{1}, nil
					}).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return results[0], nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("ignores negative concurrency limit", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(node).
					WithConcurrencyLimit(-5). // Should be ignored
					WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
						return []any{1}, nil
					}).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return results[0], nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores nil node", func(t *testing.T) {
		validNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(nil). // Should be ignored
							WithNode(validNode). // This should be used
							WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
						return []any{1}, nil
					}).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return results[0], nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores nil segmenter", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		validSegmenter := func(ctx context.Context, input any) ([]any, error) {
			return []any{1}, nil
		}

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(node).
					WithSegmenter(nil).            // Should be ignored
					WithSegmenter(validSegmenter). // This should be used
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return results[0], nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores nil aggregator", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		validAggregator := func(ctx context.Context, results []any) (any, error) {
			return results[0], nil
		}

		flow, err := NewBuilder().
			Batch(func(batch *BatchBuilder) {
				batch.WithNode(node).
					WithSegmenter(func(ctx context.Context, input any) ([]any, error) {
						return []any{1}, nil
					}).
					WithAggregator(nil).            // Should be ignored
					WithAggregator(validAggregator) // This should be used
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestAsyncBuilder tests the AsyncBuilder functionality with closure-based configuration
func TestAsyncBuilder(t *testing.T) {
	t.Run("builds async with node only", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return input.(string) + "-processed", nil
		})

		flow, err := NewBuilder().
			Async(func(async *AsyncBuilder) {
				async.WithNode(node)
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		future, ok := result.(sync.Future[any])
		if !ok {
			t.Fatalf("expected Future[any], got %T", result)
		}

		value, err := future.Get()
		if err != nil {
			t.Errorf("expected no error from future, got %v", err)
		}

		if value.(string) != "test-processed" {
			t.Errorf("expected 'test-processed', got %s", value.(string))
		}
	})

	t.Run("builds async with pool", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-pooled", nil
		})
		pool := sync.PoolOfNoPool()

		flow, err := NewBuilder().
			Async(func(async *AsyncBuilder) {
				async.WithNode(node).
					WithPool(pool)
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		future, ok := result.(sync.Future[any])
		if !ok {
			t.Fatalf("expected Future[any], got %T", result)
		}

		value, err := future.Get()
		if err != nil {
			t.Errorf("expected no error from future, got %v", err)
		}

		if value.(string) != "test-pooled" {
			t.Errorf("expected 'test-pooled', got %s", value.(string))
		}
	})

	t.Run("ignores nil node", func(t *testing.T) {
		validNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Async(func(async *AsyncBuilder) {
				async.WithNode(nil). // Should be ignored
							WithNode(validNode) // This should be used
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		future := result.(sync.Future[any])
		value, _ := future.Get()
		if value.(string) != "test" {
			t.Errorf("expected 'test', got %s", value.(string))
		}
	})

	t.Run("ignores nil pool", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Async(func(async *AsyncBuilder) {
				async.WithNode(node).
					WithPool(nil) // Should be ignored, uses default
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestParallelBuilder tests the ParallelBuilder functionality with closure-based configuration
func TestParallelBuilder(t *testing.T) {
	t.Run("builds parallel with all options", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-1", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-2", nil
		})

		flow, err := NewBuilder().
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(node1, node2).
					WithWaitAll().
					WithContinueOnError().
					WithCancelRemaining().
					WithRequiredSuccesses(2).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return fmt.Sprintf("%v,%v", results[0], results[1]), nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		resultStr := result.(string)
		if !strings.Contains(resultStr, "test-1") || !strings.Contains(resultStr, "test-2") {
			t.Errorf("expected result to contain both outputs, got %s", resultStr)
		}
	})

	t.Run("builds parallel with wait any", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			time.Sleep(100 * time.Millisecond)
			return "slow", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "fast", nil
		})

		flow, err := NewBuilder().
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(node1, node2).
					WithWaitAny().
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						for _, r := range results {
							if r != nil {
								return r, nil
							}
						}
						return nil, errors.New("no results")
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "fast" {
			t.Errorf("expected 'fast', got %s", result.(string))
		}
	})

	t.Run("builds parallel with wait count", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(node, node, node).
					WithWaitCount(2).
					WithAggregator(func(ctx context.Context, results []any) (any, error) {
						count := 0
						for _, r := range results {
							if r != nil {
								count++
							}
						}
						return count, nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) < 2 {
			t.Errorf("expected at least 2 results, got %d", result.(int))
		}
	})

	t.Run("ignores empty nodes", func(t *testing.T) {
		validNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewBuilder().
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(). // Should be ignored
							WithNodes(validNode). // This should be used
							WithAggregator(func(ctx context.Context, results []any) (any, error) {
						return results[0], nil
					})
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ignores nil aggregator", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		validAggregator := func(ctx context.Context, results []any) (any, error) {
			return results[0], nil
		}

		flow, err := NewBuilder().
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(node).
					WithAggregator(nil).            // Should be ignored
					WithAggregator(validAggregator) // This should be used
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		_, err = flow.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestBuilder tests the main Builder functionality with closure-based configuration
func TestBuilder(t *testing.T) {
	t.Run("creates new builder", func(t *testing.T) {
		b := NewBuilder()
		if b == nil {
			t.Fatal("expected non-nil builder")
		}
		if b.nodes != nil {
			t.Error("expected nodes to be nil initially")
		}
		if b.errs != nil {
			t.Error("expected errors to be nil initially")
		}
	})

	t.Run("then adds node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		b := NewBuilder().Then(node)
		if len(b.nodes) != 1 {
			t.Errorf("expected 1 node, got %d", len(b.nodes))
		}
	})

	t.Run("then ignores nil node", func(t *testing.T) {
		b := NewBuilder().Then(nil)
		if len(b.nodes) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(b.nodes))
		}
	})

	t.Run("then chains multiple nodes", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		b := NewBuilder().Then(node1).Then(node2)
		if len(b.nodes) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(b.nodes))
		}
	})

	t.Run("build creates flow", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		flow, err := NewBuilder().Then(node).Build()
		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 10 {
			t.Errorf("expected 10, got %d", result.(int))
		}
	})

	t.Run("build fails with no nodes", func(t *testing.T) {
		_, err := NewBuilder().Build()
		if err == nil {
			t.Error("expected error for empty builder")
		}
	})

	t.Run("build can only be called once", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		b := NewBuilder().Then(node)
		_, err := b.Build()
		if err != nil {
			t.Fatalf("first build failed: %v", err)
		}

		_, err = b.Build()
		if err == nil {
			t.Error("expected error on second build")
		}
		if !strings.Contains(err.Error(), "already built") {
			t.Errorf("expected 'already built' error, got %v", err)
		}
	})

	t.Run("cannot modify after build", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		b := NewBuilder().Then(node)
		_, err := b.Build()
		if err != nil {
			t.Fatalf("build failed: %v", err)
		}

		// Try to add another node
		b.Then(node)
		if len(b.errs) == 0 {
			t.Error("expected error to be recorded")
		}
	})

	t.Run("complex workflow integration", func(t *testing.T) {
		// Initial processing
		initNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})

		// Loop to multiply
		loopNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		// Branch decision
		branchDecision := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		highBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) - 10, nil
		})
		lowBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})

		// Final processing
		finalNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		flow, err := NewBuilder().
			Then(initNode).
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(loopNode).
					WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
						return iteration < 2, nil
					})
			}).
			Branch(func(branch *BranchBuilder) {
				branch.WithNode(branchDecision).
					WithBranch("high", highBranch).
					WithBranch("low", lowBranch).
					WithBranchResolver(func(ctx context.Context, input, output any) (string, error) {
						if input.(int) > 50 {
							return "high", nil
						}
						return "low", nil
					})
			}).
			Then(finalNode).
			Build()

		if err != nil {
			t.Fatalf("failed to build complex flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 80 {
			t.Errorf("expected 80, got %d", result.(int))
		}
	})

	t.Run("mixed Then and closure-based builders", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 1, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})

		flow, err := NewBuilder().
			Then(node1).
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node2).
					WithMaxIterations(2)
			}).
			Then(node3).
			Build()

		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 22 {
			t.Errorf("expected 22, got %d", result.(int))
		}
	})
}

// Benchmark tests
func BenchmarkBuilder_SimpleBuild(b *testing.B) {
	node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewBuilder().Then(node).Build()
	}
}

func BenchmarkBuilder_ComplexBuild(b *testing.B) {
	node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) + 1, nil
	})
	node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})
	aggregator := func(ctx context.Context, results []any) (any, error) {
		sum := 0
		for _, r := range results {
			sum += r.(int)
		}
		return sum, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewBuilder().
			Then(node1).
			Parallel(func(parallel *ParallelBuilder) {
				parallel.WithNodes(node2, node2).
					WithAggregator(aggregator)
			}).
			Build()
	}
}

func BenchmarkBuilder_ClosureOverhead(b *testing.B) {
	node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input, nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewBuilder().
			Loop(func(loop *LoopBuilder) {
				loop.WithNode(node).WithMaxIterations(1)
			})
	}
}
