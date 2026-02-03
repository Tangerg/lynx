package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestFunc_Run tests the basic execution of Func
func TestFunc_Run(t *testing.T) {
	tests := []struct {
		name    string
		fn      Func[int, int]
		input   int
		want    int
		wantErr bool
		errMsg  string
	}{
		{
			name: "simple function",
			fn: func(ctx context.Context, input int) (int, error) {
				return input * 2, nil
			},
			input:   5,
			want:    10,
			wantErr: false,
		},
		{
			name: "function returning error",
			fn: func(ctx context.Context, input int) (int, error) {
				if input < 0 {
					return 0, errors.New("negative input not allowed")
				}
				return input, nil
			},
			input:   -5,
			want:    0,
			wantErr: true,
			errMsg:  "negative input not allowed",
		},
		{
			name:    "nil function",
			fn:      nil,
			input:   10,
			want:    0,
			wantErr: true,
			errMsg:  "cannot run nil function: func is not initialized",
		},
		{
			name: "function with zero input",
			fn: func(ctx context.Context, input int) (int, error) {
				return input + 100, nil
			},
			input:   0,
			want:    100,
			wantErr: false,
		},
		{
			name: "identity function",
			fn: func(ctx context.Context, input int) (int, error) {
				return input, nil
			},
			input:   42,
			want:    42,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := tt.fn.Run(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Run() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
			}

			if got != tt.want {
				t.Errorf("Run() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFunc_DifferentTypes tests Func with various type combinations
func TestFunc_DifferentTypes(t *testing.T) {
	t.Run("int to string", func(t *testing.T) {
		fn := Func[int, string](func(ctx context.Context, input int) (string, error) {
			return fmt.Sprintf("number: %d", input), nil
		})

		result, err := fn.Run(context.Background(), 42)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "number: 42" {
			t.Errorf("Run() got = %v, want 'number: 42'", result)
		}
	})

	t.Run("string to int", func(t *testing.T) {
		fn := Func[string, int](func(ctx context.Context, input string) (int, error) {
			return len(input), nil
		})

		result, err := fn.Run(context.Background(), "hello")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != 5 {
			t.Errorf("Run() got = %v, want 5", result)
		}
	})

	t.Run("string to string", func(t *testing.T) {
		fn := Func[string, string](func(ctx context.Context, input string) (string, error) {
			return strings.ToUpper(input), nil
		})

		result, err := fn.Run(context.Background(), "hello")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "HELLO" {
			t.Errorf("Run() got = %v, want HELLO", result)
		}
	})

	t.Run("struct to struct", func(t *testing.T) {
		type Input struct {
			Value int
		}
		type Output struct {
			Result int
		}

		fn := Func[Input, Output](func(ctx context.Context, input Input) (Output, error) {
			return Output{Result: input.Value * 2}, nil
		})

		result, err := fn.Run(context.Background(), Input{Value: 21})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result.Result != 42 {
			t.Errorf("Run() got = %v, want 42", result.Result)
		}
	})

	t.Run("slice to slice", func(t *testing.T) {
		fn := Func[[]int, []int](func(ctx context.Context, input []int) ([]int, error) {
			result := make([]int, len(input))
			for i, v := range input {
				result[i] = v * 2
			}
			return result, nil
		})

		result, err := fn.Run(context.Background(), []int{1, 2, 3})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		expected := []int{2, 4, 6}
		if len(result) != len(expected) {
			t.Errorf("Run() result length = %v, want %v", len(result), len(expected))
		}
		for i := range result {
			if result[i] != expected[i] {
				t.Errorf("Run() result[%d] = %v, want %v", i, result[i], expected[i])
			}
		}
	})

	t.Run("map to map", func(t *testing.T) {
		fn := Func[map[string]int, map[string]int](func(ctx context.Context, input map[string]int) (map[string]int, error) {
			result := make(map[string]int)
			for k, v := range input {
				result[k] = v * 2
			}
			return result, nil
		})

		result, err := fn.Run(context.Background(), map[string]int{"a": 1, "b": 2})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result["a"] != 2 || result["b"] != 4 {
			t.Errorf("Run() got = %v, want {a:2, b:4}", result)
		}
	})
}

// TestFunc_WithContext tests context handling in Func
func TestFunc_WithContext(t *testing.T) {
	t.Run("context value access", func(t *testing.T) {
		type contextKey string
		key := contextKey("user_id")

		fn := Func[string, string](func(ctx context.Context, input string) (string, error) {
			userID := ctx.Value(key)
			if userID == nil {
				return "", errors.New("user_id not found in context")
			}
			return fmt.Sprintf("%s: %s", userID, input), nil
		})

		ctx := context.WithValue(context.Background(), key, "user123")
		result, err := fn.Run(ctx, "hello")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "user123: hello" {
			t.Errorf("Run() got = %v, want 'user123: hello'", result)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
				return input * 2, nil
			}
		})

		_, err := fn.Run(ctx, 10)
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error should be context.Canceled, got %v", err)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			select {
			case <-time.After(50 * time.Millisecond):
				return input * 2, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		})

		_, err := fn.Run(ctx, 10)
		if err == nil {
			t.Errorf("Run() expected timeout error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Run() error should be context.DeadlineExceeded, got %v", err)
		}
	})

	t.Run("context with deadline", func(t *testing.T) {
		deadline := time.Now().Add(100 * time.Millisecond)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			if d, ok := ctx.Deadline(); ok {
				if time.Until(d) < 0 {
					return 0, errors.New("deadline exceeded")
				}
			}
			return input * 2, nil
		})

		result, err := fn.Run(ctx, 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != 10 {
			t.Errorf("Run() got = %v, want 10", result)
		}
	})
}

// TestFunc_ErrorHandling tests various error scenarios
func TestFunc_ErrorHandling(t *testing.T) {
	t.Run("custom error types", func(t *testing.T) {
		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			if input < 0 {
				return 0, errors.New("negative input not allowed")
			}
			return input, nil
		})

		_, err := fn.Run(context.Background(), -1)
		if err == nil {
			t.Errorf("Run() expected validation error")
		}
	})

	t.Run("wrapped errors", func(t *testing.T) {
		baseErr := errors.New("base error")
		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 0 {
				return 0, fmt.Errorf("processing failed: %w", baseErr)
			}
			return input, nil
		})

		_, err := fn.Run(context.Background(), 0)
		if err == nil {
			t.Errorf("Run() expected error")
		}
		if !errors.Is(err, baseErr) {
			t.Errorf("Run() error should wrap baseErr")
		}
	})

	t.Run("panic recovery", func(t *testing.T) {
		fn := Func[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 0 {
				panic("division by zero")
			}
			return 100 / input, nil
		})

		// Note: Current implementation doesn't recover from panics
		// This test documents the behavior
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Run() should panic but didn't")
			}
		}()

		fn.Run(context.Background(), 0)
	})
}

// TestFunc_AsNode tests that Func can be used as a Node
func TestFunc_AsNode(t *testing.T) {
	t.Run("implements Node interface", func(t *testing.T) {
		// This test verifies that Func[I, O] satisfies the Node interface
		var _ interface {
			Run(context.Context, int) (int, error)
		} = Func[int, int](func(ctx context.Context, i int) (int, error) {
			return i * 2, nil
		})
	})

	t.Run("can be used in composition", func(t *testing.T) {
		// Demonstrates that Func can be composed with other nodes
		double := Func[int, int](func(ctx context.Context, x int) (int, error) {
			return x * 2, nil
		})

		addTen := Func[int, int](func(ctx context.Context, x int) (int, error) {
			return x + 10, nil
		})

		ctx := context.Background()
		result, err := double.Run(ctx, 5)
		if err != nil {
			t.Fatalf("double.Run() error = %v", err)
		}

		result, err = addTen.Run(ctx, result)
		if err != nil {
			t.Fatalf("addTen.Run() error = %v", err)
		}

		if result != 20 {
			t.Errorf("composed result = %v, want 20", result)
		}
	})
}

// TestFunc_ComplexScenarios tests real-world usage patterns
func TestFunc_ComplexScenarios(t *testing.T) {
	t.Run("data validation", func(t *testing.T) {
		type User struct {
			Name  string
			Email string
			Age   int
		}

		validate := Func[User, User](func(ctx context.Context, user User) (User, error) {
			if user.Name == "" {
				return User{}, errors.New("name is required")
			}
			if !strings.Contains(user.Email, "@") {
				return User{}, errors.New("invalid email")
			}
			if user.Age < 0 || user.Age > 150 {
				return User{}, errors.New("invalid age")
			}
			return user, nil
		})

		validUser := User{Name: "John", Email: "john@example.com", Age: 30}
		result, err := validate.Run(context.Background(), validUser)
		if err != nil {
			t.Errorf("validate() error = %v", err)
		}
		if result != validUser {
			t.Errorf("validate() got = %v, want %v", result, validUser)
		}

		invalidUser := User{Name: "", Email: "invalid", Age: -5}
		_, err = validate.Run(context.Background(), invalidUser)
		if err == nil {
			t.Errorf("validate() expected error for invalid user")
		}
	})

	t.Run("data transformation", func(t *testing.T) {
		type Request struct {
			RawData string
		}
		type Response struct {
			ProcessedData string
			Length        int
		}

		transform := Func[Request, Response](func(ctx context.Context, req Request) (Response, error) {
			processed := strings.ToUpper(strings.TrimSpace(req.RawData))
			return Response{
				ProcessedData: processed,
				Length:        len(processed),
			}, nil
		})

		input := Request{RawData: "  hello world  "}
		result, err := transform.Run(context.Background(), input)
		if err != nil {
			t.Errorf("transform() error = %v", err)
		}
		if result.ProcessedData != "HELLO WORLD" {
			t.Errorf("transform() ProcessedData = %v, want 'HELLO WORLD'", result.ProcessedData)
		}
		if result.Length != 11 {
			t.Errorf("transform() Length = %v, want 11", result.Length)
		}
	})

	t.Run("async operation simulation", func(t *testing.T) {
		fetchData := Func[string, string](func(ctx context.Context, id string) (string, error) {
			// Simulate async data fetch
			select {
			case <-time.After(10 * time.Millisecond):
				return fmt.Sprintf("data_%s", id), nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})

		ctx := context.Background()
		result, err := fetchData.Run(ctx, "123")
		if err != nil {
			t.Errorf("fetchData() error = %v", err)
		}
		if result != "data_123" {
			t.Errorf("fetchData() got = %v, want 'data_123'", result)
		}
	})

	t.Run("filter operation", func(t *testing.T) {
		filterEven := Func[[]int, []int](func(ctx context.Context, input []int) ([]int, error) {
			result := make([]int, 0)
			for _, v := range input {
				if v%2 == 0 {
					result = append(result, v)
				}
			}
			return result, nil
		})

		input := []int{1, 2, 3, 4, 5, 6}
		result, err := filterEven.Run(context.Background(), input)
		if err != nil {
			t.Errorf("filterEven() error = %v", err)
		}
		expected := []int{2, 4, 6}
		if len(result) != len(expected) {
			t.Errorf("filterEven() result length = %v, want %v", len(result), len(expected))
		}
		for i := range result {
			if result[i] != expected[i] {
				t.Errorf("filterEven() result[%d] = %v, want %v", i, result[i], expected[i])
			}
		}
	})

	t.Run("aggregate operation", func(t *testing.T) {
		sum := Func[[]int, int](func(ctx context.Context, input []int) (int, error) {
			total := 0
			for _, v := range input {
				total += v
			}
			return total, nil
		})

		result, err := sum.Run(context.Background(), []int{1, 2, 3, 4, 5})
		if err != nil {
			t.Errorf("sum() error = %v", err)
		}
		if result != 15 {
			t.Errorf("sum() got = %v, want 15", result)
		}
	})
}

// TestFunc_Chaining demonstrates function composition
func TestFunc_Chaining(t *testing.T) {
	t.Run("manual chaining", func(t *testing.T) {
		step1 := Func[int, int](func(ctx context.Context, x int) (int, error) {
			return x * 2, nil
		})

		step2 := Func[int, int](func(ctx context.Context, x int) (int, error) {
			return x + 10, nil
		})

		step3 := Func[int, string](func(ctx context.Context, x int) (string, error) {
			return fmt.Sprintf("result: %d", x), nil
		})

		ctx := context.Background()

		result1, err := step1.Run(ctx, 5)
		if err != nil {
			t.Fatalf("step1 error = %v", err)
		}

		result2, err := step2.Run(ctx, result1)
		if err != nil {
			t.Fatalf("step2 error = %v", err)
		}

		result3, err := step3.Run(ctx, result2)
		if err != nil {
			t.Fatalf("step3 error = %v", err)
		}

		if result3 != "result: 20" {
			t.Errorf("final result = %v, want 'result: 20'", result3)
		}
	})
}

// TestFunc_NilHandling tests nil function handling
func TestFunc_NilHandling(t *testing.T) {
	t.Run("nil function with int type", func(t *testing.T) {
		var fn Func[int, int]
		result, err := fn.Run(context.Background(), 10)
		if err == nil {
			t.Errorf("Run() expected error for nil function")
		}
		if result != 0 {
			t.Errorf("Run() result = %v, want 0 (zero value)", result)
		}
		if !strings.Contains(err.Error(), "cannot run nil function") {
			t.Errorf("Run() error message = %v, want 'cannot run nil function'", err.Error())
		}
	})

	t.Run("nil function with string type", func(t *testing.T) {
		var fn Func[string, string]
		result, err := fn.Run(context.Background(), "test")
		if err == nil {
			t.Errorf("Run() expected error for nil function")
		}
		if result != "" {
			t.Errorf("Run() result = %v, want empty string (zero value)", result)
		}
	})

	t.Run("nil function with struct type", func(t *testing.T) {
		type Data struct {
			Value int
		}
		var fn Func[Data, Data]
		result, err := fn.Run(context.Background(), Data{Value: 10})
		if err == nil {
			t.Errorf("Run() expected error for nil function")
		}
		if result.Value != 0 {
			t.Errorf("Run() result.Value = %v, want 0 (zero value)", result.Value)
		}
	})
}
