package flow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestNewSequence tests the constructor for Sequence
func TestNewSequence(t *testing.T) {
	tests := []struct {
		name       string
		processors []func(context.Context, any) (any, error)
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid single processor",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			},
			wantErr: false,
		},
		{
			name: "valid multiple processors",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "no processors",
			processors: []func(context.Context, any) (any, error){},
			wantErr:    true,
			errMsg:     "at least one processor is required",
		},
		{
			name:       "nil processors slice",
			processors: nil,
			wantErr:    true,
			errMsg:     "at least one processor is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, err := NewSequence(tt.processors...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewSequence() expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewSequence() error = %v, want %v", err.Error(), tt.errMsg)
				}
				if seq != nil {
					t.Errorf("NewSequence() expected nil sequence on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewSequence() unexpected error = %v", err)
				}
				if seq == nil {
					t.Errorf("NewSequence() returned nil sequence")
				}
				if len(seq.processors) != len(tt.processors) {
					t.Errorf("NewSequence() processors count = %v, want %v",
						len(seq.processors), len(tt.processors))
				}
			}
		})
	}
}

// TestSequence_Run tests the basic execution logic
func TestSequence_Run(t *testing.T) {
	tests := []struct {
		name       string
		processors []func(context.Context, any) (any, error)
		input      any
		want       any
		wantErr    bool
		errSubstr  string
	}{
		{
			name: "single processor - identity",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			},
			input:   42,
			want:    42,
			wantErr: false,
		},
		{
			name: "single processor - transformation",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
			},
			input:   10,
			want:    20,
			wantErr: false,
		},
		{
			name: "two processors - int operations",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				},
			},
			input:   5,
			want:    20, // (5 * 2) + 10 = 20
			wantErr: false,
		},
		{
			name: "three processors - int chain",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 3, nil
				},
			},
			input:   5,
			want:    60, // ((5 * 2) + 10) * 3 = 60
			wantErr: false,
		},
		{
			name: "type conversion - int to string",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return fmt.Sprintf("result: %d", input.(int)), nil
				},
			},
			input:   5,
			want:    "result: 10",
			wantErr: false,
		},
		{
			name: "type conversion chain",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return strconv.Itoa(input.(int)), nil
				},
				func(ctx context.Context, input any) (any, error) {
					return strings.ToUpper(input.(string)), nil
				},
				func(ctx context.Context, input any) (any, error) {
					return len(input.(string)), nil
				},
			},
			input:   42,
			want:    2, // "42" -> "42" (len=2)
			wantErr: false,
		},
		{
			name: "error in first processor",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return nil, errors.New("first processor failed")
				},
				func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			},
			input:     10,
			wantErr:   true,
			errSubstr: "sequence failed at processor 0",
		},
		{
			name: "error in middle processor",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return nil, errors.New("middle processor failed")
				},
				func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				},
			},
			input:     10,
			wantErr:   true,
			errSubstr: "sequence failed at processor 1",
		},
		{
			name: "error in last processor",
			processors: []func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				func(ctx context.Context, input any) (any, error) {
					return nil, errors.New("last processor failed")
				},
			},
			input:     10,
			wantErr:   true,
			errSubstr: "sequence failed at processor 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, err := NewSequence(tt.processors...)
			if err != nil {
				t.Fatalf("NewSequence() error = %v", err)
			}

			ctx := context.Background()
			got, err := seq.Run(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error but got nil")
				} else if tt.errSubstr != "" {
					if !strings.Contains(err.Error(), tt.errSubstr) {
						t.Errorf("Run() error = %v, want substring %v", err.Error(), tt.errSubstr)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
				if got != tt.want {
					t.Errorf("Run() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestSequence_RunWithContext tests context handling
func TestSequence_RunWithContext(t *testing.T) {
	t.Run("context passed to all processors", func(t *testing.T) {
		type contextKey string
		key := contextKey("counter")
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				count := ctx.Value(key).(int)
				return input.(int) + count, nil
			},
			func(ctx context.Context, input any) (any, error) {
				count := ctx.Value(key).(int)
				return input.(int) * count, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		ctx := context.WithValue(context.Background(), key, 10)
		result, err := seq.Run(ctx, 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// (5 + 10) * 10 = 150
		if result.(int) != 150 {
			t.Errorf("Run() got = %v, want 150", result)
		}
	})

	t.Run("context cancellation in first processor", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return input, nil
				}
			},
			func(ctx context.Context, input any) (any, error) {
				return input, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, err = seq.Run(ctx, 10)
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Run() error should contain context cancellation")
		}
	})

	t.Run("context cancellation in middle processor", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			},
			func(ctx context.Context, input any) (any, error) {
				cancel() // Cancel here
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return input, nil
				}
			},
			func(ctx context.Context, input any) (any, error) {
				return input.(int) + 10, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, err = seq.Run(ctx, 5)
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return input, nil
			},
			func(ctx context.Context, input any) (any, error) {
				select {
				case <-time.After(50 * time.Millisecond):
					return input, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, err = seq.Run(ctx, 10)
		if err == nil {
			t.Errorf("Run() expected timeout error")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline exceeded") {
			t.Errorf("Run() error should be timeout related, got %v", err)
		}
	})
}

// TestSequence_ErrorPropagation tests error handling and propagation
func TestSequence_ErrorPropagation(t *testing.T) {
	t.Run("error includes processor index", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			},
			func(ctx context.Context, input any) (any, error) {
				return input.(int) + 10, nil
			},
			func(ctx context.Context, input any) (any, error) {
				return nil, errors.New("processor error")
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, err = seq.Run(context.Background(), 5)
		if err == nil {
			t.Fatalf("Run() expected error")
		}

		if !strings.Contains(err.Error(), "processor 2") {
			t.Errorf("error should mention processor index 2, got: %v", err.Error())
		}
	})

	t.Run("wrapped error preservation", func(t *testing.T) {
		baseErr := errors.New("base error")
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return nil, fmt.Errorf("wrapped: %w", baseErr)
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, err = seq.Run(context.Background(), 5)
		if err == nil {
			t.Fatalf("Run() expected error")
		}

		if !errors.Is(err, baseErr) {
			t.Errorf("error should wrap baseErr")
		}
	})

	t.Run("subsequent processors not executed after error", func(t *testing.T) {
		executionOrder := []int{}

		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				executionOrder = append(executionOrder, 0)
				return input, nil
			},
			func(ctx context.Context, input any) (any, error) {
				executionOrder = append(executionOrder, 1)
				return nil, errors.New("error in processor 1")
			},
			func(ctx context.Context, input any) (any, error) {
				executionOrder = append(executionOrder, 2)
				return input, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		_, _ = seq.Run(context.Background(), 5)

		if len(executionOrder) != 2 {
			t.Errorf("expected 2 processors to execute, got %d", len(executionOrder))
		}
		if executionOrder[0] != 0 || executionOrder[1] != 1 {
			t.Errorf("execution order = %v, want [0, 1]", executionOrder)
		}
	})
}

// TestSequence_ComplexScenarios tests real-world usage patterns
func TestSequence_ComplexScenarios(t *testing.T) {
	t.Run("data processing pipeline", func(t *testing.T) {
		type Data struct {
			Value string
			Count int
		}

		processors := []func(context.Context, any) (any, error){
			// Validate
			func(ctx context.Context, input any) (any, error) {
				data := input.(Data)
				if data.Value == "" {
					return nil, errors.New("value is empty")
				}
				return data, nil
			},
			// Transform
			func(ctx context.Context, input any) (any, error) {
				data := input.(Data)
				data.Value = strings.ToUpper(data.Value)
				return data, nil
			},
			// Enrich
			func(ctx context.Context, input any) (any, error) {
				data := input.(Data)
				data.Count = len(data.Value)
				return data, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		input := Data{Value: "hello", Count: 0}
		result, err := seq.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		output := result.(Data)
		if output.Value != "HELLO" {
			t.Errorf("output.Value = %v, want HELLO", output.Value)
		}
		if output.Count != 5 {
			t.Errorf("output.Count = %v, want 5", output.Count)
		}
	})

	t.Run("mathematical computation chain", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return input.(float64) * 2.0, nil
			},
			func(ctx context.Context, input any) (any, error) {
				return input.(float64) + 3.5, nil
			},
			func(ctx context.Context, input any) (any, error) {
				return input.(float64) / 2.0, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		result, err := seq.Run(context.Background(), 5.0)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// ((5.0 * 2.0) + 3.5) / 2.0 = 6.75
		expected := 6.75
		if result.(float64) != expected {
			t.Errorf("result = %v, want %v", result, expected)
		}
	})

	t.Run("string manipulation pipeline", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return strings.TrimSpace(input.(string)), nil
			},
			func(ctx context.Context, input any) (any, error) {
				return strings.ToLower(input.(string)), nil
			},
			func(ctx context.Context, input any) (any, error) {
				return strings.ReplaceAll(input.(string), " ", "_"), nil
			},
			func(ctx context.Context, input any) (any, error) {
				return "processed_" + input.(string), nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		result, err := seq.Run(context.Background(), "  Hello World  ")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result.(string) != "processed_hello_world" {
			t.Errorf("result = %v, want 'processed_hello_world'", result)
		}
	})

	t.Run("slice processing pipeline", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			// Filter out zeros
			func(ctx context.Context, input any) (any, error) {
				nums := input.([]int)
				result := []int{}
				for _, n := range nums {
					if n != 0 {
						result = append(result, n)
					}
				}
				return result, nil
			},
			// Double each value
			func(ctx context.Context, input any) (any, error) {
				nums := input.([]int)
				result := make([]int, len(nums))
				for i, n := range nums {
					result[i] = n * 2
				}
				return result, nil
			},
			// Sum
			func(ctx context.Context, input any) (any, error) {
				nums := input.([]int)
				sum := 0
				for _, n := range nums {
					sum += n
				}
				return sum, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		result, err := seq.Run(context.Background(), []int{1, 0, 2, 3, 0, 4})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Filter: [1, 2, 3, 4]
		// Double: [2, 4, 6, 8]
		// Sum: 20
		if result.(int) != 20 {
			t.Errorf("result = %v, want 20", result)
		}
	})

	t.Run("map transformation pipeline", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			// Add prefix to keys
			func(ctx context.Context, input any) (any, error) {
				m := input.(map[string]int)
				result := make(map[string]int)
				for k, v := range m {
					result["key_"+k] = v
				}
				return result, nil
			},
			// Double values
			func(ctx context.Context, input any) (any, error) {
				m := input.(map[string]int)
				result := make(map[string]int)
				for k, v := range m {
					result[k] = v * 2
				}
				return result, nil
			},
			// Count entries
			func(ctx context.Context, input any) (any, error) {
				m := input.(map[string]int)
				return len(m), nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		input := map[string]int{"a": 1, "b": 2, "c": 3}
		result, err := seq.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result.(int) != 3 {
			t.Errorf("result = %v, want 3", result)
		}
	})
}

// TestSequence_TypeAssertionPanics tests panic recovery (documents current behavior)
func TestSequence_TypeAssertionPanics(t *testing.T) {
	t.Run("type assertion panic", func(t *testing.T) {
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				// This will panic if input is not an int
				return input.(int) * 2, nil
			},
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		// Note: Current implementation doesn't recover from panics
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Run() should panic for type mismatch but didn't")
			}
		}()

		seq.Run(context.Background(), "not an int")
	})
}

// TestSequence_LongChain tests performance with many processors
func TestSequence_LongChain(t *testing.T) {
	t.Run("100 processors", func(t *testing.T) {
		processors := make([]func(context.Context, any) (any, error), 100)
		for i := range processors {
			processors[i] = func(ctx context.Context, input any) (any, error) {
				return input.(int) + 1, nil
			}
		}

		seq, err := NewSequence(processors...)
		if err != nil {
			t.Fatalf("NewSequence() error = %v", err)
		}

		result, err := seq.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result.(int) != 100 {
			t.Errorf("result = %v, want 100", result)
		}
	})
}
