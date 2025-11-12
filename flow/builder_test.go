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

// TestLoopBuilder tests the LoopBuilder functionality
// TestLoopBuilder tests the LoopBuilder functionality
func TestLoopBuilder(t *testing.T) {
	t.Run("creates loop builder with new builder", func(t *testing.T) {
		lb := NewLoopBuilder()
		if lb == nil {
			t.Fatal("expected non-nil loop builder")
		}
		if lb.builder == nil {
			t.Error("expected non-nil parent builder")
		}
		if lb.config == nil {
			t.Error("expected non-nil config")
		}
	})

	t.Run("creates loop builder with existing builder", func(t *testing.T) {
		b := NewBuilder()
		lb := NewLoopBuilder(b)
		if lb == nil {
			t.Fatal("expected non-nil loop builder")
		}
		if lb.builder != b {
			t.Error("expected parent builder to be the same")
		}
	})

	t.Run("with node sets node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		lb := NewLoopBuilder().WithNode(node)
		if lb.config.Node == nil {
			t.Error("expected node to be set")
		}
	})

	t.Run("with node ignores nil", func(t *testing.T) {
		lb := NewLoopBuilder().WithNode(nil)
		if lb.config.Node != nil {
			t.Error("expected node to remain nil")
		}
	})

	t.Run("with max iterations sets value", func(t *testing.T) {
		lb := NewLoopBuilder().WithMaxIterations(10)
		if lb.config.MaxIterations != 10 {
			t.Errorf("expected MaxIterations to be 10, got %d", lb.config.MaxIterations)
		}
	})

	t.Run("with max iterations ignores zero", func(t *testing.T) {
		lb := NewLoopBuilder().WithMaxIterations(0)
		if lb.config.MaxIterations != 0 {
			t.Errorf("expected MaxIterations to be 0, got %d", lb.config.MaxIterations)
		}
	})

	t.Run("with max iterations ignores negative", func(t *testing.T) {
		lb := NewLoopBuilder().WithMaxIterations(-5)
		if lb.config.MaxIterations != 0 {
			t.Errorf("expected MaxIterations to remain 0, got %d", lb.config.MaxIterations)
		}
	})

	t.Run("with terminator sets terminator", func(t *testing.T) {
		terminator := func(ctx context.Context, iteration int, input, output any) (bool, error) {
			return false, nil
		}

		lb := NewLoopBuilder().WithTerminator(terminator)
		if lb.config.Terminator == nil {
			t.Error("expected terminator to be set")
		}
	})

	t.Run("with terminator ignores nil", func(t *testing.T) {
		lb := NewLoopBuilder().WithTerminator(nil)
		if lb.config.Terminator != nil {
			t.Error("expected terminator to remain nil")
		}
	})

	t.Run("then builds loop with node only", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		b := NewLoopBuilder().
			WithNode(node).
			Then()

		if b == nil {
			t.Fatal("expected non-nil builder")
		}

		flow, err := b.Build()
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

	t.Run("then builds loop with terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})
		terminator := func(ctx context.Context, iteration int, input, output any) (bool, error) {
			return iteration >= 2, nil // Stop after 3 iterations (0, 1, 2)
		}

		b := NewLoopBuilder().
			WithNode(node).
			WithTerminator(terminator).
			Then()

		if b == nil {
			t.Fatal("expected non-nil builder")
		}

		flow, err := b.Build()
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

	t.Run("then builds loop with max iterations only", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + counter, nil
		})

		b := NewLoopBuilder().
			WithNode(node).
			WithMaxIterations(5).
			Then()

		if b == nil {
			t.Fatal("expected non-nil builder")
		}

		flow, err := b.Build()
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

	t.Run("then builds loop with both max iterations and terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})
		terminator := func(ctx context.Context, iteration int, input, output any) (bool, error) {
			return iteration >= 2, nil // Would stop at iteration 2
		}

		b := NewLoopBuilder().
			WithNode(node).
			WithMaxIterations(10).
			WithTerminator(terminator).
			Then()

		if b == nil {
			t.Fatal("expected non-nil builder")
		}

		flow, err := b.Build()
		if err != nil {
			t.Fatalf("failed to build: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// With AND logic: stops when iteration >= 2 AND terminator returns true
		if counter != 3 {
			t.Errorf("expected 3 executions, got %d", counter)
		}

		if result.(int) != 1 {
			t.Errorf("expected 1, got %d", result.(int))
		}
	})

	t.Run("then builds loop with max iterations reached before terminator", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})
		terminator := func(ctx context.Context, iteration int, input, output any) (bool, error) {
			return false, nil // Never terminate
		}

		b := NewLoopBuilder().
			WithNode(node).
			WithMaxIterations(3).
			WithTerminator(terminator).
			Then()

		if b == nil {
			t.Fatal("expected non-nil builder")
		}

		flow, err := b.Build()
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

	t.Run("method chaining", func(t *testing.T) {
		counter := 0
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			counter++
			return input.(int) + 1, nil
		})

		lb := NewLoopBuilder()
		result := lb.
			WithNode(node).
			WithMaxIterations(5).
			WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
				return iteration >= 2, nil
			})

		if result != lb {
			t.Error("expected method chaining to return same builder instance")
		}
	})

	t.Run("then records error on invalid config - nil node", func(t *testing.T) {
		b := NewLoopBuilder().Then()
		_, err := b.Build()
		if err == nil {
			t.Error("expected error for invalid loop config")
		}

		if err.Error() != "loop node cannot be nil" {
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

		flow, err := NewLoopBuilder().
			WithNode(node1).
			WithMaxIterations(3).
			WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
				return iteration >= 1, nil
			}).
			Then().
			Loop().
			WithNode(node2).
			WithMaxIterations(2).
			WithTerminator(func(ctx context.Context, iteration int, input, output any) (bool, error) {
				return iteration >= 1, nil
			}).
			Then().
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

		// First loop: 1
		// Second loop: 2
		if result.(int) != 2 {
			t.Errorf("expected 2, got %d", result.(int))
		}
	})
}

// TestBranchBuilder tests the BranchBuilder functionality
func TestBranchBuilder(t *testing.T) {
	t.Run("creates branch builder with new builder", func(t *testing.T) {
		bb := NewBranchBuilder()
		if bb == nil {
			t.Fatal("expected non-nil branch builder")
		}
		if bb.builder == nil {
			t.Error("expected non-nil parent builder")
		}
		if bb.config == nil {
			t.Error("expected non-nil config")
		}
		if bb.config.Branches == nil {
			t.Error("expected non-nil branches map")
		}
	})

	t.Run("creates branch builder with existing builder", func(t *testing.T) {
		b := NewBuilder()
		bb := NewBranchBuilder(b)
		if bb.builder != b {
			t.Error("expected parent builder to be the same")
		}
	})

	t.Run("with node sets node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "decision", nil
		})

		bb := NewBranchBuilder().WithNode(node)
		if bb.config.Node == nil {
			t.Error("expected node to be set")
		}
	})

	t.Run("with node ignores nil", func(t *testing.T) {
		bb := NewBranchBuilder().WithNode(nil)
		if bb.config.Node != nil {
			t.Error("expected node to remain nil")
		}
	})

	t.Run("with branch adds single branch", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch result", nil
		})

		bb := NewBranchBuilder().WithBranch("test", node)
		if len(bb.config.Branches) != 1 {
			t.Errorf("expected 1 branch, got %d", len(bb.config.Branches))
		}
		if bb.config.Branches["test"] == nil {
			t.Error("expected branch 'test' to be set")
		}
	})

	t.Run("with branch ignores nil node", func(t *testing.T) {
		bb := NewBranchBuilder().WithBranch("test", nil)
		if len(bb.config.Branches) != 0 {
			t.Errorf("expected 0 branches, got %d", len(bb.config.Branches))
		}
	})

	t.Run("with branches sets all branches", func(t *testing.T) {
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

		bb := NewBranchBuilder().WithBranches(branches)
		if len(bb.config.Branches) != 2 {
			t.Errorf("expected 2 branches, got %d", len(bb.config.Branches))
		}
	})

	t.Run("with branches ignores nil", func(t *testing.T) {
		bb := NewBranchBuilder().
			WithBranch("existing", Processor[any, any](func(ctx context.Context, input any) (any, error) {
				return nil, nil
			})).
			WithBranches(nil)

		if len(bb.config.Branches) != 1 {
			t.Errorf("expected branches to remain unchanged")
		}
	})

	t.Run("with branch resolver sets resolver", func(t *testing.T) {
		resolver := func(ctx context.Context, input, output any) (string, error) {
			return "branch1", nil
		}

		bb := NewBranchBuilder().WithBranchResolver(resolver)
		if bb.config.BranchResolver == nil {
			t.Error("expected resolver to be set")
		}
	})

	t.Run("with branch resolver ignores nil", func(t *testing.T) {
		bb := NewBranchBuilder().WithBranchResolver(nil)
		if bb.config.BranchResolver != nil {
			t.Error("expected resolver to remain nil")
		}
	})

	t.Run("then builds and adds to parent", func(t *testing.T) {
		decisionNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "success", nil
		})
		successNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "success result", nil
		})
		failureNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "failure result", nil
		})
		resolver := func(ctx context.Context, input, output any) (string, error) {
			return output.(string), nil
		}

		b := NewBranchBuilder().
			WithNode(decisionNode).
			WithBranch("success", successNode).
			WithBranch("failure", failureNode).
			WithBranchResolver(resolver).
			Then()

		flow, err := b.Build()
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
}

// TestBatchBuilder tests the BatchBuilder functionality
func TestBatchBuilder(t *testing.T) {
	t.Run("creates batch builder with new builder", func(t *testing.T) {
		bb := NewBatchBuilder()
		if bb == nil {
			t.Fatal("expected non-nil batch builder")
		}
		if bb.builder == nil {
			t.Error("expected non-nil parent builder")
		}
		if bb.config == nil {
			t.Error("expected non-nil config")
		}
	})

	t.Run("with continue on error sets flag", func(t *testing.T) {
		bb := NewBatchBuilder().WithContinueOnError()
		if !bb.config.ContinueOnError {
			t.Error("expected ContinueOnError to be true")
		}
	})

	t.Run("with concurrency limit sets limit", func(t *testing.T) {
		bb := NewBatchBuilder().WithConcurrencyLimit(5)
		if bb.config.ConcurrencyLimit != 5 {
			t.Errorf("expected concurrency limit 5, got %d", bb.config.ConcurrencyLimit)
		}
	})

	t.Run("with concurrency limit ignores non-positive", func(t *testing.T) {
		bb := NewBatchBuilder().WithConcurrencyLimit(0)
		if bb.config.ConcurrencyLimit != 0 {
			t.Error("expected concurrency limit to remain 0")
		}
	})

	t.Run("with node sets node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		bb := NewBatchBuilder().WithNode(node)
		if bb.config.Node == nil {
			t.Error("expected node to be set")
		}
	})

	t.Run("with segmenter sets segmenter", func(t *testing.T) {
		segmenter := func(ctx context.Context, input any) ([]any, error) {
			return []any{1, 2, 3}, nil
		}

		bb := NewBatchBuilder().WithSegmenter(segmenter)
		if bb.config.Segmenter == nil {
			t.Error("expected segmenter to be set")
		}
	})

	t.Run("with aggregator sets aggregator", func(t *testing.T) {
		aggregator := func(ctx context.Context, results []any) (any, error) {
			return len(results), nil
		}

		bb := NewBatchBuilder().WithAggregator(aggregator)
		if bb.config.Aggregator == nil {
			t.Error("expected aggregator to be set")
		}
	})

	t.Run("then builds and adds to parent", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		segmenter := func(ctx context.Context, input any) ([]any, error) {
			arr := input.([]int)
			result := make([]any, len(arr))
			for i, v := range arr {
				result[i] = v
			}
			return result, nil
		}
		aggregator := func(ctx context.Context, results []any) (any, error) {
			sum := 0
			for _, r := range results {
				sum += r.(int)
			}
			return sum, nil
		}

		b := NewBatchBuilder().
			WithNode(node).
			WithSegmenter(segmenter).
			WithAggregator(aggregator).
			WithConcurrencyLimit(2).
			Then()

		flow, err := b.Build()
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
}

// TestAsyncBuilder tests the AsyncBuilder functionality
func TestAsyncBuilder(t *testing.T) {
	t.Run("creates async builder with new builder", func(t *testing.T) {
		ab := NewAsyncBuilder()
		if ab == nil {
			t.Fatal("expected non-nil async builder")
		}
		if ab.builder == nil {
			t.Error("expected non-nil parent builder")
		}
		if ab.config == nil {
			t.Error("expected non-nil config")
		}
	})

	t.Run("with node sets node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		ab := NewAsyncBuilder().WithNode(node)
		if ab.config.Node == nil {
			t.Error("expected node to be set")
		}
	})

	t.Run("with pool sets pool", func(t *testing.T) {
		pool := sync.PoolOfNoPool()
		ab := NewAsyncBuilder().WithPool(pool)
		if ab.config.Pool == nil {
			t.Error("expected pool to be set")
		}
	})

	t.Run("with pool ignores nil", func(t *testing.T) {
		ab := NewAsyncBuilder().WithPool(nil)
		if ab.config.Pool != nil {
			t.Error("expected pool to remain nil")
		}
	})

	t.Run("then builds and adds to parent", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return input.(string) + "-processed", nil
		})

		b := NewAsyncBuilder().
			WithNode(node).
			Then()

		flow, err := b.Build()
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
}

// TestParallelBuilder tests the ParallelBuilder functionality
func TestParallelBuilder(t *testing.T) {
	t.Run("creates parallel builder with new builder", func(t *testing.T) {
		pb := NewParallelBuilder()
		if pb == nil {
			t.Fatal("expected non-nil parallel builder")
		}
		if pb.builder == nil {
			t.Error("expected non-nil parent builder")
		}
		if pb.config == nil {
			t.Error("expected non-nil config")
		}
	})

	t.Run("with wait count sets wait count", func(t *testing.T) {
		pb := NewParallelBuilder().WithWaitCount(3)
		if pb.config.WaitCount != 3 {
			t.Errorf("expected wait count 3, got %d", pb.config.WaitCount)
		}
	})

	t.Run("with wait any sets wait count to 1", func(t *testing.T) {
		pb := NewParallelBuilder().WithWaitAny()
		if pb.config.WaitCount != 1 {
			t.Errorf("expected wait count 1, got %d", pb.config.WaitCount)
		}
	})

	t.Run("with wait all sets wait count to -1", func(t *testing.T) {
		pb := NewParallelBuilder().WithWaitAll()
		if pb.config.WaitCount != -1 {
			t.Errorf("expected wait count -1, got %d", pb.config.WaitCount)
		}
	})

	t.Run("with nodes sets nodes", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result1", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "result2", nil
		})

		pb := NewParallelBuilder().WithNodes(node1, node2)
		if len(pb.config.Nodes) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(pb.config.Nodes))
		}
	})

	t.Run("with nodes ignores empty", func(t *testing.T) {
		pb := NewParallelBuilder().WithNodes()
		if pb.config.Nodes != nil {
			t.Error("expected nodes to remain nil")
		}
	})

	t.Run("with aggregator sets aggregator", func(t *testing.T) {
		aggregator := func(ctx context.Context, results []any) (any, error) {
			return len(results), nil
		}

		pb := NewParallelBuilder().WithAggregator(aggregator)
		if pb.config.Aggregator == nil {
			t.Error("expected aggregator to be set")
		}
	})

	t.Run("with cancel remaining sets flag", func(t *testing.T) {
		pb := NewParallelBuilder().WithCancelRemaining()
		if !pb.config.CancelRemaining {
			t.Error("expected CancelRemaining to be true")
		}
	})

	t.Run("with continue on error sets flag", func(t *testing.T) {
		pb := NewParallelBuilder().WithContinueOnError()
		if !pb.config.ContinueOnError {
			t.Error("expected ContinueOnError to be true")
		}
	})

	t.Run("with required successes sets count", func(t *testing.T) {
		pb := NewParallelBuilder().WithRequiredSuccesses(2)
		if pb.config.RequiredSuccesses != 2 {
			t.Errorf("expected required successes 2, got %d", pb.config.RequiredSuccesses)
		}
	})

	t.Run("then builds and adds to parent", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-1", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-2", nil
		})
		aggregator := func(ctx context.Context, results []any) (any, error) {
			return fmt.Sprintf("%v,%v", results[0], results[1]), nil
		}

		b := NewParallelBuilder().
			WithNodes(node1, node2).
			WithAggregator(aggregator).
			Then()

		flow, err := b.Build()
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
}

// TestBuilder tests the main Builder functionality
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

	t.Run("record error stores error", func(t *testing.T) {
		b := NewBuilder()
		err := errors.New("test error")
		b.recordError(err)

		if len(b.errs) != 1 {
			t.Errorf("expected 1 error, got %d", len(b.errs))
		}
	})

	t.Run("record error ignores nil", func(t *testing.T) {
		b := NewBuilder()
		b.recordError(nil)

		if len(b.errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(b.errs))
		}
	})

	t.Run("validate fails with no nodes", func(t *testing.T) {
		b := NewBuilder()
		err := b.validate()

		if err == nil {
			t.Error("expected error for no nodes")
		}
	})

	t.Run("validate fails with recorded errors", func(t *testing.T) {
		b := NewBuilder()
		b.recordError(errors.New("error 1"))
		b.recordError(errors.New("error 2"))

		err := b.validate()
		if err == nil {
			t.Error("expected error from validation")
		}
	})

	t.Run("validate succeeds with valid config", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		b := NewBuilder().Then(node)
		err := b.validate()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

	t.Run("complex workflow integration", func(t *testing.T) {
		// Initial processing
		initNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})

		// Loop to multiply
		loopNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		loopTerminator := func(ctx context.Context, iteration int, input, output any) (bool, error) {
			return iteration < 2, nil
		}

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
		branchResolver := func(ctx context.Context, input, output any) (string, error) {
			if input.(int) > 50 {
				return "high", nil
			}
			return "low", nil
		}

		// Final processing
		finalNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		flow, err := NewBuilder().
			Then(initNode).
			Loop().
			WithNode(loopNode).
			WithTerminator(loopTerminator).
			Then().
			Branch().
			WithNode(branchDecision).
			WithBranch("high", highBranch).
			WithBranch("low", lowBranch).
			WithBranchResolver(branchResolver).
			Then().
			Then(finalNode).
			Build()

		if err != nil {
			t.Fatalf("failed to build complex flow: %v", err)
		}

		// (5 + 10) * 2 = 30 -> low branch -> 30 + 10 = 40 -> 40 * 2 = 80
		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 80 {
			t.Errorf("expected 80, got %d", result.(int))
		}
	})

	t.Run("branch returns branch builder", func(t *testing.T) {
		b := NewBuilder()
		bb := b.Branch()

		if bb == nil {
			t.Fatal("expected non-nil branch builder")
		}
		if bb.builder != b {
			t.Error("expected branch builder to reference parent")
		}
	})

	t.Run("loop returns loop builder", func(t *testing.T) {
		b := NewBuilder()
		lb := b.Loop()

		if lb == nil {
			t.Fatal("expected non-nil loop builder")
		}
		if lb.builder != b {
			t.Error("expected loop builder to reference parent")
		}
	})

	t.Run("batch returns batch builder", func(t *testing.T) {
		b := NewBuilder()
		bb := b.Batch()

		if bb == nil {
			t.Fatal("expected non-nil batch builder")
		}
		if bb.builder != b {
			t.Error("expected batch builder to reference parent")
		}
	})

	t.Run("async returns async builder", func(t *testing.T) {
		b := NewBuilder()
		ab := b.Async()

		if ab == nil {
			t.Fatal("expected non-nil async builder")
		}
		if ab.builder != b {
			t.Error("expected async builder to reference parent")
		}
	})

	t.Run("parallel returns parallel builder", func(t *testing.T) {
		b := NewBuilder()
		pb := b.Parallel()

		if pb == nil {
			t.Fatal("expected non-nil parallel builder")
		}
		if pb.builder != b {
			t.Error("expected parallel builder to reference parent")
		}
	})
}

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
			Parallel().
			WithNodes(node2, node2).
			WithAggregator(aggregator).
			Then().
			Build()
	}
}
