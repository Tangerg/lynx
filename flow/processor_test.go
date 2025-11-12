package flow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessor_Run(t *testing.T) {
	t.Run("successful execution with string types", func(t *testing.T) {
		processor := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "processed: " + input, nil
		})

		result, err := processor.Run(context.Background(), "test")

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("successful execution with different types", func(t *testing.T) {
		processor := Processor[int, string](func(ctx context.Context, input int) (string, error) {
			return string(rune(input + 65)), nil
		})

		result, err := processor.Run(context.Background(), 0)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != "A" {
			t.Errorf("expected 'A', got %q", result)
		}
	})

	t.Run("processor returns error", func(t *testing.T) {
		expectedErr := errors.New("processing failed")
		processor := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			return "", expectedErr
		})

		result, err := processor.Run(context.Background(), "test")

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("nil processor", func(t *testing.T) {
		var processor Processor[string, string]

		result, err := processor.Run(context.Background(), "test")

		if err == nil {
			t.Error("expected error for nil processor, got nil")
		}

		if err.Error() != "processor cannot be nil" {
			t.Errorf("expected 'processor cannot be nil' error, got %v", err)
		}

		if result != "" {
			t.Errorf("expected zero value (empty string), got %q", result)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		processor := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "completed", nil
			}
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result, err := processor.Run(ctx, "test")

		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got %v", err)
		}

		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		processor := Processor[string, string](func(ctx context.Context, input string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return "completed", nil
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		result, err := processor.Run(ctx, "test")

		if err != context.DeadlineExceeded {
			t.Errorf("expected context.DeadlineExceeded error, got %v", err)
		}

		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("processor with struct types", func(t *testing.T) {
		type Input struct {
			Name string
			Age  int
		}
		type Output struct {
			Message string
		}

		processor := Processor[Input, Output](func(ctx context.Context, input Input) (Output, error) {
			return Output{
				Message: input.Name + " is " + string(rune(input.Age+48)),
			}, nil
		})

		input := Input{Name: "Alice", Age: 2}
		result, err := processor.Run(context.Background(), input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.Message != "Alice is 2" {
			t.Errorf("expected 'Alice is 2', got %q", result.Message)
		}
	})

	t.Run("processor with pointer types", func(t *testing.T) {
		processor := Processor[*string, *int](func(ctx context.Context, input *string) (*int, error) {
			if input == nil {
				return nil, errors.New("input cannot be nil")
			}
			length := len(*input)
			return &length, nil
		})

		input := "hello"
		result, err := processor.Run(context.Background(), &input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if *result != 5 {
			t.Errorf("expected 5, got %d", *result)
		}
	})

	t.Run("processor with nil pointer input", func(t *testing.T) {
		processor := Processor[*string, *int](func(ctx context.Context, input *string) (*int, error) {
			if input == nil {
				return nil, errors.New("input cannot be nil")
			}
			length := len(*input)
			return &length, nil
		})

		result, err := processor.Run(context.Background(), nil)

		if err == nil {
			t.Error("expected error for nil input, got nil")
		}

		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("zero value return on error", func(t *testing.T) {
		type ComplexType struct {
			Data  []string
			Count int
		}

		processor := Processor[string, ComplexType](func(ctx context.Context, input string) (ComplexType, error) {
			return ComplexType{}, errors.New("some error")
		})

		result, err := processor.Run(context.Background(), "test")

		if err == nil {
			t.Error("expected error, got nil")
		}

		if result.Data != nil || result.Count != 0 {
			t.Errorf("expected zero value, got %+v", result)
		}
	})
}

func BenchmarkProcessor_Run(b *testing.B) {
	processor := Processor[string, string](func(ctx context.Context, input string) (string, error) {
		return "processed: " + input, nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = processor.Run(ctx, "test")
	}
}

func BenchmarkProcessor_Run_WithComplexType(b *testing.B) {
	type Input struct {
		Values []int
	}
	type Output struct {
		Sum int
	}

	processor := Processor[Input, Output](func(ctx context.Context, input Input) (Output, error) {
		sum := 0
		for _, v := range input.Values {
			sum += v
		}
		return Output{Sum: sum}, nil
	})

	input := Input{Values: []int{1, 2, 3, 4, 5}}
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = processor.Run(ctx, input)
	}
}

func BenchmarkProcessor_Run_NilCheck(b *testing.B) {
	var processor Processor[string, string]
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = processor.Run(ctx, "test")
	}
}
