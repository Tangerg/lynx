package flow

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNode_Interface ensures Node interface can be implemented
func TestNode_Interface(t *testing.T) {
	t.Run("processor implements node", func(t *testing.T) {
		processor := Processor[string, int](func(ctx context.Context, input string) (int, error) {
			return len(input), nil
		})

		var _ Node[string, int] = processor
	})
}

// TestMiddleware_Concept tests middleware patterns
func TestMiddleware_Concept(t *testing.T) {
	t.Run("logging middleware", func(t *testing.T) {
		var loggedInput string
		var loggedOutput string

		loggingMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				loggedInput = input
				output, err := node.Run(ctx, input)
				loggedOutput = output
				return output, err
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input + "-processed", nil
		})

		wrappedNode := loggingMiddleware(baseNode)

		result, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "test-processed" {
			t.Errorf("expected 'test-processed', got %s", result)
		}

		if loggedInput != "test" {
			t.Errorf("expected logged input 'test', got %s", loggedInput)
		}

		if loggedOutput != "test-processed" {
			t.Errorf("expected logged output 'test-processed', got %s", loggedOutput)
		}
	})

	t.Run("timing middleware", func(t *testing.T) {
		var executionTime time.Duration

		timingMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				start := time.Now()
				output, err := node.Run(ctx, input)
				executionTime = time.Since(start)
				return output, err
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return input, nil
		})

		wrappedNode := timingMiddleware(baseNode)

		_, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if executionTime < 10*time.Millisecond {
			t.Errorf("expected execution time >= 10ms, got %v", executionTime)
		}
	})

	t.Run("retry middleware", func(t *testing.T) {
		attempts := 0
		maxRetries := 3

		retryMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				var lastErr error
				for i := 0; i < maxRetries; i++ {
					output, err := node.Run(ctx, input)
					if err == nil {
						return output, nil
					}
					lastErr = err
				}
				return "", lastErr
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			attempts++
			if attempts < 3 {
				return "", errors.New("temporary error")
			}
			return input + "-success", nil
		})

		wrappedNode := retryMiddleware(baseNode)

		result, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "test-success" {
			t.Errorf("expected 'test-success', got %s", result)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("validation middleware", func(t *testing.T) {
		validationMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				if input == "" {
					return "", errors.New("input cannot be empty")
				}
				output, err := node.Run(ctx, input)
				if err != nil {
					return "", err
				}
				if output == "" {
					return "", errors.New("output cannot be empty")
				}
				return output, nil
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input + "-processed", nil
		})

		wrappedNode := validationMiddleware(baseNode)

		// Test with empty input
		_, err := wrappedNode.Run(context.Background(), "")
		if err == nil {
			t.Error("expected error for empty input")
		}

		// Test with valid input
		result, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "test-processed" {
			t.Errorf("expected 'test-processed', got %s", result)
		}
	})

	t.Run("chained middleware", func(t *testing.T) {
		var logged bool
		var timed bool

		loggingMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				logged = true
				return node.Run(ctx, input)
			})
		}

		timingMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				timed = true
				return node.Run(ctx, input)
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		// Apply middleware in order
		wrappedNode := loggingMiddleware(timingMiddleware(baseNode))

		_, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !logged {
			t.Error("expected logging middleware to execute")
		}

		if !timed {
			t.Error("expected timing middleware to execute")
		}
	})

	t.Run("error handling middleware", func(t *testing.T) {
		errorHandlingMiddleware := func(node Node[string, string]) Node[string, string] {
			return Processor[string, string](func(ctx context.Context, input string) (string, error) {
				output, err := node.Run(ctx, input)
				if err != nil {
					// Handle error and return default value
					return "error-handled", nil
				}
				return output, nil
			})
		}

		baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "", errors.New("node error")
		})

		wrappedNode := errorHandlingMiddleware(baseNode)

		result, err := wrappedNode.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "error-handled" {
			t.Errorf("expected 'error-handled', got %s", result)
		}
	})
}

// TestJoin tests the Join function
func TestJoin(t *testing.T) {
	t.Run("joins single node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		joined, err := Join(node)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		result, err := joined.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 10 {
			t.Errorf("expected 10, got %d", result.(int))
		}
	})

	t.Run("joins multiple nodes", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) - 5, nil
		})

		joined, err := Join(node1, node2, node3)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// (5 + 10) * 2 - 5 = 25
		result, err := joined.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 25 {
			t.Errorf("expected 25, got %d", result.(int))
		}
	})

	t.Run("joins no nodes returns error", func(t *testing.T) {
		_, err := Join()
		if err == nil {
			t.Error("expected error for no nodes")
		}

		if err.Error() != "no nodes provided" {
			t.Errorf("expected 'no nodes provided' error, got %v", err)
		}
	})

	t.Run("joined nodes execute in sequence", func(t *testing.T) {
		var executionOrder []int

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 1)
			return input, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 2)
			return input, nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 3)
			return input, nil
		})

		joined, err := Join(node1, node2, node3)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		_, err = joined.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expectedOrder := []int{1, 2, 3}
		if len(executionOrder) != len(expectedOrder) {
			t.Errorf("expected %d executions, got %d", len(expectedOrder), len(executionOrder))
		}

		for i, order := range expectedOrder {
			if executionOrder[i] != order {
				t.Errorf("execution order[%d]: expected %d, got %d", i, order, executionOrder[i])
			}
		}
	})

	t.Run("joined nodes stop on error", func(t *testing.T) {
		var executionOrder []int

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 1)
			return input, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 2)
			return nil, errors.New("node2 error")
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 3)
			return input, nil
		})

		joined, err := Join(node1, node2, node3)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		_, err = joined.Run(context.Background(), "test")
		if err == nil {
			t.Error("expected error from node2")
		}

		if len(executionOrder) != 2 {
			t.Errorf("expected 2 executions, got %d", len(executionOrder))
		}
	})

	t.Run("joined nodes propagate data", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-step1", nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-step2", nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-step3", nil
		})

		joined, err := Join(node1, node2, node3)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		result, err := joined.Run(context.Background(), "start")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "start-step1-step2-step3"
		if result.(string) != expected {
			t.Errorf("expected %s, got %s", expected, result.(string))
		}
	})

	t.Run("joined nodes respect context", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return input, nil
			}
		})

		joined, err := Join(node1, node2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = joined.Run(ctx, "test")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})
}

func BenchmarkMiddleware_Logging(b *testing.B) {
	loggingMiddleware := func(node Node[string, string]) Node[string, string] {
		return Processor[string, string](func(ctx context.Context, input string) (string, error) {
			// Simulate logging
			_ = input
			output, err := node.Run(ctx, input)
			_ = output
			return output, err
		})
	}

	baseNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
		return input + "-processed", nil
	})

	wrappedNode := loggingMiddleware(baseNode)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = wrappedNode.Run(ctx, "test")
	}
}

func BenchmarkJoin_MultipleNodes(b *testing.B) {
	nodes := make([]Node[any, any], 5)
	for i := 0; i < 5; i++ {
		nodes[i] = Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 1, nil
		})
	}

	joined, _ := Join(nodes...)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = joined.Run(ctx, 0)
	}
}
