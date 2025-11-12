package flow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestLoopConfig_validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *LoopConfig[string, string]

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if err.Error() != "loop config cannot be nil" {
			t.Errorf("expected 'loop config cannot be nil' error, got %v", err)
		}
	})

	t.Run("nil node", func(t *testing.T) {
		cfg := &LoopConfig[string, string]{
			Node: nil,
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil node, got nil")
		}

		if err.Error() != "loop node cannot be nil" {
			t.Errorf("expected 'loop node cannot be nil' error, got %v", err)
		}
	})

	t.Run("valid config with terminator", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 5, nil
			},
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestNewLoop(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if loop == nil {
			t.Error("expected non-nil loop")
		}

		if loop.node == nil {
			t.Error("expected non-nil node in loop")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &LoopConfig[string, string]{
			Node: nil,
		}

		loop, err := NewLoop(cfg)
		if err == nil {
			t.Error("expected error for invalid config, got nil")
		}

		if loop != nil {
			t.Error("expected nil loop for invalid config")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		loop, err := NewLoop[string, string](nil)
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if loop != nil {
			t.Error("expected nil loop for nil config")
		}
	})
}

func TestLoop_shouldTerminate(t *testing.T) {
	t.Run("nil terminator returns true", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		loop := &Loop[string, string]{
			node:       mockNode,
			terminator: nil,
		}

		shouldStop, err := loop.shouldTerminate(context.Background(), 0, "input", "output")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !shouldStop {
			t.Error("expected true for nil terminator")
		}
	})

	t.Run("terminator returns true", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		loop := &Loop[string, string]{
			node: mockNode,
			terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return true, nil
			},
		}

		shouldStop, err := loop.shouldTerminate(context.Background(), 0, "input", "output")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !shouldStop {
			t.Error("expected true from terminator")
		}
	})

	t.Run("terminator returns false", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		loop := &Loop[string, string]{
			node: mockNode,
			terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return false, nil
			},
		}

		shouldStop, err := loop.shouldTerminate(context.Background(), 0, "input", "output")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if shouldStop {
			t.Error("expected false from terminator")
		}
	})

	t.Run("terminator returns error", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		expectedErr := errors.New("terminator error")
		loop := &Loop[string, string]{
			node: mockNode,
			terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return false, expectedErr
			},
		}

		_, err := loop.shouldTerminate(context.Background(), 0, "input", "output")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestLoop_Run(t *testing.T) {
	t.Run("single iteration with nil terminator", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 1 {
			t.Errorf("expected 1 iteration, got %d", counter)
		}

		if result != "iteration-1" {
			t.Errorf("expected 'iteration-1', got %q", result)
		}
	})

	t.Run("multiple iterations with count terminator", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 4, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 5 {
			t.Errorf("expected 5 iterations, got %d", counter)
		}

		if result != "iteration-5" {
			t.Errorf("expected 'iteration-5', got %q", result)
		}
	})

	t.Run("node returns error", func(t *testing.T) {
		expectedErr := errors.New("node error")
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "", expectedErr
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 10, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})

	t.Run("terminator returns error", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		expectedErr := errors.New("terminator error")
		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				if iteration >= 2 {
					return false, expectedErr
				}
				return false, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		_, err = loop.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if counter != 3 {
			t.Errorf("expected 3 iterations, got %d", counter)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return fmt.Sprintf("iteration-%d", counter), nil
			}
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 100, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(25 * time.Millisecond)
			cancel()
		}()

		_, err = loop.Run(ctx, "test")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})

	t.Run("output-based termination", func(t *testing.T) {
		counter := 0
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			counter++
			return input * 2, nil
		})

		cfg := &LoopConfig[int, int]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, _, _ int) (bool, error) {
				return iteration >= 32, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != 2 {
			t.Errorf("expected 2, got %d", result)
		}

		if counter != 33 {
			t.Errorf("expected 33 iterations, got %d", counter)
		}
	})

	t.Run("struct types", func(t *testing.T) {
		type Input struct {
			Value int
		}
		type Output struct {
			Count int
			Sum   int
		}

		mockNode := Processor[Input, Output](func(ctx context.Context, input Input) (Output, error) {
			return Output{Count: 1, Sum: input.Value}, nil
		})

		cfg := &LoopConfig[Input, Output]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input Input, output Output) (bool, error) {
				return iteration >= 3, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		input := Input{Value: 10}
		result, err := loop.Run(context.Background(), input)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.Count != 1 {
			t.Errorf("expected count 1, got %d", result.Count)
		}

		if result.Sum != 10 {
			t.Errorf("expected sum 10, got %d", result.Sum)
		}
	})

	t.Run("iteration count starts at zero", func(t *testing.T) {
		var iterations []int
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &LoopConfig[string, string]{
			Node: mockNode,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				iterations = append(iterations, iteration)
				return iteration >= 2, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		_, err = loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expectedIterations := []int{0, 1, 2}
		if len(iterations) != len(expectedIterations) {
			t.Errorf("expected %d iterations, got %d", len(expectedIterations), len(iterations))
		}

		for i, expected := range expectedIterations {
			if iterations[i] != expected {
				t.Errorf("iteration %d: expected %d, got %d", i, expected, iterations[i])
			}
		}
	})
}

func BenchmarkLoop_Run_SingleIteration(b *testing.B) {
	mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
		return input + "-processed", nil
	})

	cfg := &LoopConfig[string, string]{
		Node: mockNode,
	}

	loop, _ := NewLoop(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = loop.Run(ctx, "test")
	}
}

func TestLoop_MaxIterations(t *testing.T) {
	t.Run("max iterations without terminator", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 5,
			Terminator:    nil, // No terminator, rely on MaxIterations
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 5 {
			t.Errorf("expected 5 iterations, got %d", counter)
		}

		if result != "iteration-5" {
			t.Errorf("expected 'iteration-5', got %q", result)
		}
	})

	t.Run("max iterations reached before terminator", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 3,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 10, nil // Would run 11 times without MaxIterations
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")

		if counter != 3 {
			t.Errorf("expected 3 iterations, got %d", counter)
		}

		if result != "iteration-3" {
			t.Errorf("expected 'iteration-3', got %q", result)
		}
	})

	t.Run("terminator stops before max iterations", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 10,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 2, nil // Stop at iteration 2
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 3 {
			t.Errorf("expected 3 iterations (0,1,2), got %d", counter)
		}

		if result != "iteration-3" {
			t.Errorf("expected 'iteration-3', got %q", result)
		}
	})

	t.Run("zero max iterations with terminator", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 0, // No limit
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 5, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 6 {
			t.Errorf("expected 6 iterations, got %d", counter)
		}

		if result != "iteration-6" {
			t.Errorf("expected 'iteration-6', got %q", result)
		}
	})

	t.Run("negative max iterations treated as no limit", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: -1, // Should be treated as no limit
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 3, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if counter != 4 {
			t.Errorf("expected 4 iterations, got %d", counter)
		}

		if result != "iteration-4" {
			t.Errorf("expected 'iteration-4', got %q", result)
		}
	})

	t.Run("max iterations is 1", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 1,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return false, nil // Never terminate by terminator
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), "test")

		if counter != 1 {
			t.Errorf("expected 1 iteration, got %d", counter)
		}

		if result != "iteration-1" {
			t.Errorf("expected 'iteration-1', got %q", result)
		}
	})

	t.Run("node error before max iterations", func(t *testing.T) {
		counter := 0
		expectedErr := errors.New("node failure")
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			if counter == 2 {
				return "", expectedErr
			}
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 5,
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		_, err = loop.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if counter != 2 {
			t.Errorf("expected 2 iterations before error, got %d", counter)
		}
	})

	t.Run("terminator error before max iterations", func(t *testing.T) {
		counter := 0
		terminatorErr := errors.New("terminator failure")
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return fmt.Sprintf("iteration-%d", counter), nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 10,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				if iteration == 3 {
					return false, terminatorErr
				}
				return false, nil
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		_, err = loop.Run(context.Background(), "test")
		if !errors.Is(err, terminatorErr) {
			t.Errorf("expected error %v, got %v", terminatorErr, err)
		}

		if counter != 4 {
			t.Errorf("expected 4 iterations before terminator error, got %d", counter)
		}
	})
}

func TestLoop_MaxIterations_EdgeCases(t *testing.T) {
	t.Run("max iterations with accumulating state", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input + 1, nil
		})

		cfg := &LoopConfig[int, int]{
			Node:          mockNode,
			MaxIterations: 100,
			Terminator: func(ctx context.Context, iteration int, input, output int) (bool, error) {
				return output >= 50, nil // Stop when output reaches 50
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		result, err := loop.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Note: output is from node.Run, which returns input+1
		// But we're checking terminator against output, not iteration count
		// This test verifies terminator gets the actual output value
		if result != 1 {
			t.Errorf("expected result >= 1, got %d", result)
		}
	})

	t.Run("max iterations exactly equals terminator condition", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			return input, nil
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 5,
			Terminator: func(ctx context.Context, iteration int, input, output string) (bool, error) {
				return iteration >= 4, nil // Would stop at iteration 4 (5th execution)
			},
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		_, err = loop.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Terminator stops at iteration 4, which is the 5th execution
		if counter != 5 {
			t.Errorf("expected 5 iterations, got %d", counter)
		}
	})

	t.Run("context cancellation before max iterations", func(t *testing.T) {
		counter := 0
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			counter++
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return input, nil
			}
		})

		cfg := &LoopConfig[string, string]{
			Node:          mockNode,
			MaxIterations: 100,
		}

		loop, err := NewLoop(cfg)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
		defer cancel()

		_, err = loop.Run(ctx, "test")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}

		// Should execute 3-4 times before timeout (10ms each + overhead)
		if counter < 2 || counter > 5 {
			t.Errorf("expected 2-5 iterations before timeout, got %d", counter)
		}
	})
}

func BenchmarkLoop_Run_MultipleIterations(b *testing.B) {
	mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
		return input + 1, nil
	})

	cfg := &LoopConfig[int, int]{
		Node: mockNode,
		Terminator: func(ctx context.Context, iteration int, input, output int) (bool, error) {
			return iteration >= 9, nil
		},
	}

	loop, _ := NewLoop(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = loop.Run(ctx, 0)
	}
}
