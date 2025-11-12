package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelConfig_validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input, nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{mockNode, mockNode},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty nodes", func(t *testing.T) {
		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for empty nodes, got nil")
		}

		expectedMsg := "parallel must contain at least one node"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("nil aggregator", func(t *testing.T) {
		mockNode := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input, nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes:      []Node[string, any]{mockNode},
			Aggregator: nil,
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil aggregator, got nil")
		}

		expectedMsg := "parallel must have aggregator"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})
}

func TestNewParallel(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input, nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{mockNode, mockNode},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
			WaitCount:         1,
			RequiredSuccesses: 1,
			ContinueOnError:   true,
			CancelRemaining:   true,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if parallel == nil {
			t.Error("expected non-nil parallel")
		}

		if len(parallel.nodes) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(parallel.nodes))
		}

		if parallel.waitCount != 1 {
			t.Errorf("expected waitCount 1, got %d", parallel.waitCount)
		}

		if parallel.requiredSuccesses != 1 {
			t.Errorf("expected requiredSuccesses 1, got %d", parallel.requiredSuccesses)
		}

		if !parallel.continueOnError {
			t.Error("expected continueOnError to be true")
		}

		if !parallel.cancelRemaining {
			t.Error("expected cancelRemaining to be true")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{},
		}

		parallel, err := NewParallel(cfg)
		if err == nil {
			t.Error("expected error for invalid config, got nil")
		}

		if parallel != nil {
			t.Error("expected nil parallel for invalid config")
		}
	})
}

func TestParallel_getWaitCount(t *testing.T) {
	t.Run("positive wait count less than node count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:     make([]Node[string, any], 5),
			waitCount: 3,
		}

		count := parallel.getWaitCount()
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("wait count exceeds node count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:     make([]Node[string, any], 3),
			waitCount: 5,
		}

		count := parallel.getWaitCount()
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("zero wait count defaults to all nodes", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:     make([]Node[string, any], 5),
			waitCount: 0,
		}

		count := parallel.getWaitCount()
		if count != 5 {
			t.Errorf("expected 5, got %d", count)
		}
	})

	t.Run("negative wait count defaults to all nodes", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:     make([]Node[string, any], 5),
			waitCount: -1,
		}

		count := parallel.getWaitCount()
		if count != 5 {
			t.Errorf("expected 5, got %d", count)
		}
	})
}

func TestParallel_getRequiredSuccesses(t *testing.T) {
	t.Run("positive required successes less than wait count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 5),
			waitCount:         4,
			requiredSuccesses: 2,
		}

		count := parallel.getRequiredSuccesses()
		if count != 2 {
			t.Errorf("expected 2, got %d", count)
		}
	})

	t.Run("required successes exceeds wait count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 5),
			waitCount:         3,
			requiredSuccesses: 5,
		}

		count := parallel.getRequiredSuccesses()
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("zero required successes defaults to wait count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 5),
			waitCount:         3,
			requiredSuccesses: 0,
		}

		count := parallel.getRequiredSuccesses()
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("negative required successes defaults to wait count", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 5),
			waitCount:         4,
			requiredSuccesses: -1,
		}

		count := parallel.getRequiredSuccesses()
		if count != 4 {
			t.Errorf("expected 4, got %d", count)
		}
	})
}

func TestParallel_validateResults(t *testing.T) {
	t.Run("sufficient successful results", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 3),
			requiredSuccesses: 2,
		}

		successfulResults := []any{"result1", "result2"}
		var collectedErrors []error

		results, err := parallel.validateResults(successfulResults, collectedErrors)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("insufficient successful results", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 3),
			requiredSuccesses: 2,
		}

		successfulResults := []any{"result1"}
		collectedErrors := []error{errors.New("error1")}

		results, err := parallel.validateResults(successfulResults, collectedErrors)
		if err == nil {
			t.Error("expected error for insufficient results, got nil")
		}

		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}

		expectedMsg := "insufficient successful results"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("validation error includes collected errors", func(t *testing.T) {
		parallel := &Parallel[string, string]{
			nodes:             make([]Node[string, any], 3),
			requiredSuccesses: 2,
		}

		var successfulResults []any
		collectedErrors := []error{
			errors.New("error1"),
			errors.New("error2"),
		}

		_, err := parallel.validateResults(successfulResults, collectedErrors)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "error1") || !strings.Contains(errMsg, "error2") {
			t.Errorf("expected error to contain both errors, got %q", errMsg)
		}
	})
}

func TestParallel_Run(t *testing.T) {
	t.Run("all nodes succeed", func(t *testing.T) {
		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input + "-1", nil
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input + "-2", nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return fmt.Sprintf("%v,%v", results[0], results[1]), nil
			},
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		result, err := parallel.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !strings.Contains(result, "test-1") || !strings.Contains(result, "test-2") {
			t.Errorf("expected result to contain both outputs, got %q", result)
		}
	})

	t.Run("wait for first N completions", func(t *testing.T) {
		var completedCount atomic.Int32

		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			completedCount.Add(1)
			return "result1", nil
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			completedCount.Add(1)
			time.Sleep(10 * time.Millisecond)
			return "result2", nil
		})
		node3 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			completedCount.Add(1)
			time.Sleep(20 * time.Millisecond)
			return "result3", nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2, node3},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return fmt.Sprintf("got %d results", len(results)), nil
			},
			WaitCount:       2,
			CancelRemaining: false,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		result, err := parallel.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "got 2 results" {
			t.Errorf("expected 'got 2 results', got %q", result)
		}

		// Allow time for the third node to potentially complete
		time.Sleep(30 * time.Millisecond)
	})

	t.Run("fail fast on first error", func(t *testing.T) {
		expectedErr := errors.New("node error")

		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, expectedErr
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			time.Sleep(100 * time.Millisecond)
			return "result2", nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
			ContinueOnError: false,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		_, err = parallel.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("continue on error and collect all errors", func(t *testing.T) {
		error1 := errors.New("error1")
		error2 := errors.New("error2")

		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, error1
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, error2
		})
		node3 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result3", nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2, node3},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
			ContinueOnError:   true,
			RequiredSuccesses: 2,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		_, err = parallel.Run(context.Background(), "test")
		if err == nil {
			t.Error("expected error, got nil")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "error1") || !strings.Contains(errMsg, "error2") {
			t.Errorf("expected error to contain both errors, got %q", errMsg)
		}
	})

	t.Run("required successes met with continue on error", func(t *testing.T) {
		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result1", nil
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, errors.New("error2")
		})
		node3 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result3", nil
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2, node3},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return fmt.Sprintf("got %d results", len(results)), nil
			},
			ContinueOnError:   true,
			RequiredSuccesses: 2,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		result, err := parallel.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "got 2 results" {
			t.Errorf("expected 'got 2 results', got %q", result)
		}
	})

	t.Run("cancel remaining nodes after wait count", func(t *testing.T) {
		var node3Started atomic.Bool
		var node3Canceled atomic.Bool

		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result1", nil
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result2", nil
		})
		node3 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			node3Started.Store(true)
			select {
			case <-ctx.Done():
				node3Canceled.Store(true)
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "result3", nil
			}
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2, node3},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
			WaitCount:       2,
			CancelRemaining: true,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		result, err := parallel.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "aggregated" {
			t.Errorf("expected 'aggregated', got %q", result)
		}

		time.Sleep(20 * time.Millisecond)

		if node3Started.Load() && !node3Canceled.Load() {
			t.Error("expected node3 to be canceled")
		}
	})

	t.Run("context cancellation propagates to nodes", func(t *testing.T) {
		var nodesCanceled atomic.Int32

		createNode := func() Node[string, any] {
			return Processor[string, any](func(ctx context.Context, input string) (any, error) {
				select {
				case <-ctx.Done():
					nodesCanceled.Add(1)
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return "result", nil
				}
			})
		}

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{createNode(), createNode(), createNode()},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = parallel.Run(ctx, "test")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})

	t.Run("aggregator returns error", func(t *testing.T) {
		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result1", nil
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return "result2", nil
		})

		expectedErr := errors.New("aggregator error")
		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "", expectedErr
			},
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		_, err = parallel.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("nodes execute concurrently", func(t *testing.T) {
		var mu sync.Mutex
		var executionOrder []int
		var maxConcurrent int
		var currentConcurrent int

		createNode := func(id int) Node[string, any] {
			return Processor[string, any](func(ctx context.Context, input string) (any, error) {
				mu.Lock()
				executionOrder = append(executionOrder, id)
				currentConcurrent++
				if currentConcurrent > maxConcurrent {
					maxConcurrent = currentConcurrent
				}
				mu.Unlock()

				time.Sleep(20 * time.Millisecond)

				mu.Lock()
				currentConcurrent--
				mu.Unlock()

				return fmt.Sprintf("result%d", id), nil
			})
		}

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{
				createNode(1),
				createNode(2),
				createNode(3),
			},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		_, err = parallel.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if maxConcurrent < 2 {
			t.Errorf("expected concurrent execution, max concurrent was %d", maxConcurrent)
		}
	})

	t.Run("empty aggregation with all errors", func(t *testing.T) {
		node1 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, errors.New("error1")
		})
		node2 := Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return nil, errors.New("error2")
		})

		cfg := &ParallelConfig[string, string]{
			Nodes: []Node[string, any]{node1, node2},
			Aggregator: func(ctx context.Context, results []any) (string, error) {
				return "aggregated", nil
			},
			ContinueOnError: true,
		}

		parallel, err := NewParallel(cfg)
		if err != nil {
			t.Fatalf("failed to create parallel: %v", err)
		}

		_, err = parallel.Run(context.Background(), "test")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func BenchmarkParallel_Run_AllNodesSucceed(b *testing.B) {
	nodes := make([]Node[string, any], 5)
	for i := 0; i < 5; i++ {
		nodes[i] = Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input + "-result", nil
		})
	}

	cfg := &ParallelConfig[string, string]{
		Nodes: nodes,
		Aggregator: func(ctx context.Context, results []any) (string, error) {
			return "aggregated", nil
		},
	}

	parallel, _ := NewParallel(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = parallel.Run(ctx, "test")
	}
}

func BenchmarkParallel_Run_WithWaitCount(b *testing.B) {
	nodes := make([]Node[string, any], 10)
	for i := 0; i < 10; i++ {
		nodes[i] = Processor[string, any](func(ctx context.Context, input string) (any, error) {
			return input + "-result", nil
		})
	}

	cfg := &ParallelConfig[string, string]{
		Nodes: nodes,
		Aggregator: func(ctx context.Context, results []any) (string, error) {
			return "aggregated", nil
		},
		WaitCount: 3,
	}

	parallel, _ := NewParallel(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = parallel.Run(ctx, "test")
	}
}
