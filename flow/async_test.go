package flow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/pkg/sync"
)

func TestAsyncConfig_validate(t *testing.T) {
	t.Run("valid config with pool", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		mockPool := sync.PoolOfNoPool()

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
			Pool: mockPool,
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if cfg.Pool == nil {
			t.Error("expected pool to remain set")
		}
	})

	t.Run("valid config without pool sets default", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
			Pool: nil,
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if cfg.Pool == nil {
			t.Error("expected default pool to be set")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *AsyncConfig[string, string]

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if err.Error() != "async config cannot be nil" {
			t.Errorf("expected 'async config cannot be nil' error, got %v", err)
		}
	})

	t.Run("nil node", func(t *testing.T) {
		cfg := &AsyncConfig[string, string]{
			Node: nil,
			Pool: sync.PoolOfNoPool(),
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil node, got nil")
		}

		if err.Error() != "async node cannot be nil" {
			t.Errorf("expected 'async node cannot be nil' error, got %v", err)
		}
	})
}

func TestNewAsync(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
			Pool: sync.PoolOfNoPool(),
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if async == nil {
			t.Error("expected non-nil async")
		}

		if async.node == nil {
			t.Error("expected non-nil node in async")
		}

		if async.pool == nil {
			t.Error("expected non-nil pool in async")
		}
	})

	t.Run("valid config without pool", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if async == nil {
			t.Error("expected non-nil async")
		}

		if async.pool == nil {
			t.Error("expected default pool to be set")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &AsyncConfig[string, string]{
			Node: nil,
		}

		async, err := NewAsync(cfg)
		if err == nil {
			t.Error("expected error for invalid config, got nil")
		}

		if async != nil {
			t.Error("expected nil async for invalid config")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		async, err := NewAsync[string, string](nil)
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if async != nil {
			t.Error("expected nil async for nil config")
		}
	})
}

func TestAsync_RunType(t *testing.T) {
	t.Run("successful async execution", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "processed: " + input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		future, err := async.RunType(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if future == nil {
			t.Fatal("expected non-nil future")
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("expected no error from future, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("async execution with error", func(t *testing.T) {
		expectedErr := errors.New("processing error")
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "", expectedErr
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		future, err := async.RunType(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if future == nil {
			t.Fatal("expected non-nil future")
		}

		result, err := future.Get()
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "completed", nil
			}
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		future, err := async.RunType(ctx, "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		cancel()

		result, err := future.Get()
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}

		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})

	t.Run("multiple async executions", func(t *testing.T) {
		counter := 0
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			counter++
			return input * 2, nil
		})

		cfg := &AsyncConfig[int, int]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		futures := make([]sync.Future[int], 5)
		for i := 0; i < 5; i++ {
			future, err := async.RunType(context.Background(), i+1)
			if err != nil {
				t.Errorf("iteration %d: expected no error, got %v", i, err)
			}
			futures[i] = future
		}

		expectedResults := []int{2, 4, 6, 8, 10}
		for i, future := range futures {
			result, err := future.Get()
			if err != nil {
				t.Errorf("iteration %d: expected no error, got %v", i, err)
			}

			if result != expectedResults[i] {
				t.Errorf("iteration %d: expected %d, got %d", i, expectedResults[i], result)
			}
		}

		if counter != 5 {
			t.Errorf("expected 5 executions, got %d", counter)
		}
	})

	t.Run("async with different types", func(t *testing.T) {
		type Input struct {
			Value int
		}
		type Output struct {
			Result string
		}

		mockNode := Processor[Input, Output](func(ctx context.Context, input Input) (Output, error) {
			return Output{
				Result: "value: " + string(rune(input.Value+48)),
			}, nil
		})

		cfg := &AsyncConfig[Input, Output]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		input := Input{Value: 5}
		future, err := async.RunType(context.Background(), input)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.Result != "value: 5" {
			t.Errorf("expected 'value: 5', got %q", result.Result)
		}
	})

	t.Run("future can be checked before get", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "processed: " + input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		future, err := async.RunType(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if future.IsDone() {
			t.Error("expected future to not be done immediately")
		}

		time.Sleep(100 * time.Millisecond)

		if !future.IsDone() {
			t.Error("expected future to be done after sleep")
		}

		result, err := future.Get()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestAsync_Run(t *testing.T) {
	t.Run("successful async execution returns future as any", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "processed: " + input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		result, err := async.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		future, ok := result.(sync.Future[string])
		if !ok {
			t.Fatalf("expected result to be Future[string], got %T", result)
		}

		value, err := future.Get()
		if err != nil {
			t.Errorf("expected no error from future, got %v", err)
		}

		expected := "processed: test"
		if value != expected {
			t.Errorf("expected %q, got %q", expected, value)
		}
	})

	t.Run("async execution with error", func(t *testing.T) {
		expectedErr := errors.New("processing error")
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "", expectedErr
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		result, err := async.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		future, ok := result.(sync.Future[string])
		if !ok {
			t.Fatalf("expected result to be Future[string], got %T", result)
		}

		value, err := future.Get()
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if value != "" {
			t.Errorf("expected empty value, got %q", value)
		}
	})

	t.Run("implements Node interface", func(t *testing.T) {
		mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return input, nil
		})

		cfg := &AsyncConfig[string, string]{
			Node: mockNode,
		}

		async, err := NewAsync(cfg)
		if err != nil {
			t.Fatalf("failed to create async: %v", err)
		}

		var _ Node[string, any] = async
	})
}

func BenchmarkAsync_RunType(b *testing.B) {
	mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
		return "processed: " + input, nil
	})

	cfg := &AsyncConfig[string, string]{
		Node: mockNode,
	}

	async, _ := NewAsync(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		future, _ := async.RunType(ctx, "test")
		_, _ = future.Get()
	}
}

func BenchmarkAsync_Run(b *testing.B) {
	mockNode := Processor[string, string](func(ctx context.Context, input string) (string, error) {
		return "processed: " + input, nil
	})

	cfg := &AsyncConfig[string, string]{
		Node: mockNode,
	}

	async, _ := NewAsync(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, _ := async.Run(ctx, "test")
		future := result.(sync.Future[string])
		_, _ = future.Get()
	}
}
