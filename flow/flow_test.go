package flow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewFlow(t *testing.T) {
	t.Run("valid flow with single node", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewFlow(node)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if flow == nil {
			t.Error("expected non-nil flow")
		}

		if len(flow.nodes) != 1 {
			t.Errorf("expected 1 node, got %d", len(flow.nodes))
		}
	})

	t.Run("valid flow with multiple nodes", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if flow == nil {
			t.Error("expected non-nil flow")
		}

		if len(flow.nodes) != 3 {
			t.Errorf("expected 3 nodes, got %d", len(flow.nodes))
		}
	})

	t.Run("no nodes provided", func(t *testing.T) {
		flow, err := NewFlow()
		if err == nil {
			t.Error("expected error for no nodes, got nil")
		}

		if flow != nil {
			t.Error("expected nil flow for no nodes")
		}

		if err.Error() != "no nodes provided" {
			t.Errorf("expected 'no nodes provided' error, got %v", err)
		}
	})

	t.Run("empty node slice", func(t *testing.T) {
		var nodes []Node[any, any]
		flow, err := NewFlow(nodes...)
		if err == nil {
			t.Error("expected error for empty nodes, got nil")
		}

		if flow != nil {
			t.Error("expected nil flow for empty nodes")
		}
	})
}

func TestFlow_Run(t *testing.T) {
	t.Run("single node success", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		flow, err := NewFlow(node)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 10 {
			t.Errorf("expected 10, got %d", result.(int))
		}
	})

	t.Run("sequential execution with data transformation", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 10, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) - 5, nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// (5 + 10) * 2 - 5 = 25
		expected := 25
		if result.(int) != expected {
			t.Errorf("expected %d, got %d", expected, result.(int))
		}
	})

	t.Run("type transformation through chain", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("result: %d", input.(int)), nil
		})
		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return len(input.(string)), nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// "result: 10" has length 10
		expected := 10
		if result.(int) != expected {
			t.Errorf("expected %d, got %d", expected, result.(int))
		}
	})

	t.Run("error in first node", func(t *testing.T) {
		expectedErr := errors.New("first node error")
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return nil, expectedErr
		})
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "should not execute", nil
		})

		flow, err := NewFlow(node1, node2)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("error in middle node", func(t *testing.T) {
		var executionOrder []int

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 1)
			return input, nil
		})

		expectedErr := errors.New("middle node error")
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 2)
			return nil, expectedErr
		})

		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 3)
			return "should not execute", nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}

		if len(executionOrder) != 2 {
			t.Errorf("expected 2 executions, got %d", len(executionOrder))
		}

		if executionOrder[0] != 1 || executionOrder[1] != 2 {
			t.Errorf("expected execution order [1, 2], got %v", executionOrder)
		}
	})

	t.Run("error in last node", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		expectedErr := errors.New("last node error")
		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return nil, expectedErr
		})

		flow, err := NewFlow(node1, node2)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 5)
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "completed", nil
			}
		})

		flow, err := NewFlow(node1, node2)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result, err := flow.Run(ctx, "test")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "completed", nil
			}
		})

		flow, err := NewFlow(node)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		result, err := flow.Run(ctx, "test")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded error, got %v", err)
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("sequential execution order", func(t *testing.T) {
		var executionOrder []int

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 1)
			time.Sleep(20 * time.Millisecond)
			return 1, nil
		})

		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 2)
			time.Sleep(10 * time.Millisecond)
			return 2, nil
		})

		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			executionOrder = append(executionOrder, 3)
			return 3, nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		_, err = flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []int{1, 2, 3}
		if len(executionOrder) != len(expected) {
			t.Errorf("expected %d executions, got %d", len(expected), len(executionOrder))
		}

		for i, v := range expected {
			if executionOrder[i] != v {
				t.Errorf("execution order[%d]: expected %d, got %d", i, v, executionOrder[i])
			}
		}
	})

	t.Run("nil input handling", func(t *testing.T) {
		node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			if input == nil {
				return "nil input received", nil
			}
			return input, nil
		})

		flow, err := NewFlow(node)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "nil input received" {
			t.Errorf("expected 'nil input received', got %v", result)
		}
	})

	t.Run("complex data flow", func(t *testing.T) {
		type Data struct {
			Value int
			Label string
		}

		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			d := input.(Data)
			d.Value *= 2
			return d, nil
		})

		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			d := input.(Data)
			d.Label = fmt.Sprintf("processed-%s", d.Label)
			return d, nil
		})

		node3 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			d := input.(Data)
			d.Value += 10
			return d, nil
		})

		flow, err := NewFlow(node1, node2, node3)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		input := Data{Value: 5, Label: "test"}
		result, err := flow.Run(context.Background(), input)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		output := result.(Data)
		if output.Value != 20 {
			t.Errorf("expected value 20, got %d", output.Value)
		}

		if output.Label != "processed-test" {
			t.Errorf("expected label 'processed-test', got %s", output.Label)
		}
	})

	t.Run("many nodes in sequence", func(t *testing.T) {
		nodes := make([]Node[any, any], 10)
		for i := 0; i < 10; i++ {
			nodes[i] = Processor[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) + 1, nil
			})
		}

		flow, err := NewFlow(nodes...)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(int) != 10 {
			t.Errorf("expected 10, got %d", result.(int))
		}
	})

	t.Run("empty result propagation", func(t *testing.T) {
		node1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "", nil
		})

		node2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			s := input.(string)
			if s == "" {
				return "empty", nil
			}
			return s, nil
		})

		flow, err := NewFlow(node1, node2)
		if err != nil {
			t.Fatalf("failed to create flow: %v", err)
		}

		result, err := flow.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.(string) != "empty" {
			t.Errorf("expected 'empty', got %s", result.(string))
		}
	})
}

func BenchmarkFlow_Run_SingleNode(b *testing.B) {
	node := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})

	flow, _ := NewFlow(node)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = flow.Run(ctx, 10)
	}
}

func BenchmarkFlow_Run_MultipleNodes(b *testing.B) {
	nodes := make([]Node[any, any], 5)
	for i := 0; i < 5; i++ {
		nodes[i] = Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 1, nil
		})
	}

	flow, _ := NewFlow(nodes...)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = flow.Run(ctx, 0)
	}
}

func BenchmarkFlow_Run_ManyNodes(b *testing.B) {
	nodes := make([]Node[any, any], 20)
	for i := 0; i < 20; i++ {
		nodes[i] = Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) + 1, nil
		})
	}

	flow, _ := NewFlow(nodes...)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = flow.Run(ctx, 0)
	}
}
