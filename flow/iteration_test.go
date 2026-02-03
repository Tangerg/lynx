package flow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestIterationConfig_validate tests the validation logic for IterationConfig.
// Type: IterationConfig[int, int] - validates configuration rules
func TestIterationConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *IterationConfig[int, int]
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "iteration config cannot be nil",
		},
		{
			name: "nil processor",
			config: &IterationConfig[int, int]{
				Processor:        nil,
				ConcurrencyLimit: 1,
			},
			wantErr: true,
			errMsg:  "processor cannot be nil",
		},
		{
			name: "valid config with default concurrency",
			config: &IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 0, // Should default to 1
			},
			wantErr: false,
		},
		{
			name: "valid config with explicit concurrency",
			config: &IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 5,
			},
			wantErr: false,
		},
		{
			name: "valid config with negative concurrency (unlimited)",
			config: &IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: -1,
			},
			wantErr: false,
		},
		{
			name: "valid config with continueOnError",
			config: &IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ContinueOnError:  true,
				ConcurrencyLimit: 1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error = %v", err)
				}
				// Check default concurrency is set
				if tt.config.ConcurrencyLimit == 0 {
					t.Errorf("validate() should set default concurrency, but got 0")
				}
			}
		})
	}
}

// TestNewIteration tests the constructor for Iteration.
// Type: Iteration[int, int] - verifies proper initialization
func TestNewIteration(t *testing.T) {
	tests := []struct {
		name    string
		config  IterationConfig[int, int]
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 1,
			},
			wantErr: false,
		},
		{
			name: "nil processor",
			config: IterationConfig[int, int]{
				Processor:        nil,
				ConcurrencyLimit: 1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := NewIteration(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewIteration() expected error but got nil")
				}
				if iter != nil {
					t.Errorf("NewIteration() expected nil iteration on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewIteration() unexpected error = %v", err)
				}
				if iter == nil {
					t.Errorf("NewIteration() returned nil iteration")
				}
			}
		})
	}
}

// TestIteration_calcConcurrencyLimit tests concurrency limit calculation.
// Type: Iteration[int, int] - validates dynamic concurrency adjustment
func TestIteration_calcConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name             string
		concurrencyLimit int
		inputSize        int
		want             int
	}{
		{
			name:             "sequential (limit=0)",
			concurrencyLimit: 0,
			inputSize:        10,
			want:             1,
		},
		{
			name:             "sequential (limit=1)",
			concurrencyLimit: 1,
			inputSize:        10,
			want:             1,
		},
		{
			name:             "concurrent less than input",
			concurrencyLimit: 3,
			inputSize:        10,
			want:             3,
		},
		{
			name:             "concurrent more than input",
			concurrencyLimit: 10,
			inputSize:        5,
			want:             5,
		},
		{
			name:             "unlimited concurrency",
			concurrencyLimit: -1,
			inputSize:        10,
			want:             10,
		},
		{
			name:             "unlimited with small input",
			concurrencyLimit: -1,
			inputSize:        3,
			want:             3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter := &Iteration[int, int]{
				concurrencyLimit: tt.concurrencyLimit,
			}
			input := make([]int, tt.inputSize)
			got := iter.calcConcurrencyLimit(input)
			if got != tt.want {
				t.Errorf("calcConcurrencyLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIteration_RunSequential tests sequential processing behavior.
// Type: Iteration[int, int] - validates sequential execution and error handling
func TestIteration_RunSequential(t *testing.T) {
	tests := []struct {
		name        string
		config      IterationConfig[int, int]
		input       []int
		wantResults []int
		wantErrors  []bool
		wantErr     bool
		errSubstr   string
	}{
		{
			name: "simple doubling",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 1,
			},
			input:       []int{1, 2, 3, 4, 5},
			wantResults: []int{2, 4, 6, 8, 10},
			wantErrors:  []bool{false, false, false, false, false},
			wantErr:     false,
		},
		{
			name: "with index",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val + i, nil
				},
				ConcurrencyLimit: 1,
			},
			input:       []int{10, 20, 30},
			wantResults: []int{10, 21, 32}, // 10+0, 20+1, 30+2
			wantErrors:  []bool{false, false, false},
			wantErr:     false,
		},
		{
			name: "error stops processing",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					if val < 0 {
						return 0, errors.New("negative value")
					}
					return val * 2, nil
				},
				ContinueOnError:  false,
				ConcurrencyLimit: 1,
			},
			input:     []int{1, 2, -3, 4},
			wantErr:   true,
			errSubstr: "iteration failed at index 2",
		},
		{
			name: "continue on error",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					if val < 0 {
						return 0, errors.New("negative value")
					}
					return val * 2, nil
				},
				ContinueOnError:  true,
				ConcurrencyLimit: 1,
			},
			input:       []int{1, -2, 3, -4, 5},
			wantResults: []int{2, 0, 6, 0, 10},
			wantErrors:  []bool{false, true, false, true, false},
			wantErr:     false,
		},
		{
			name: "empty input",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 1,
			},
			input:       []int{},
			wantResults: []int{},
			wantErrors:  []bool{},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := NewIteration(tt.config)
			if err != nil {
				t.Fatalf("NewIteration() error = %v", err)
			}

			ctx := context.Background()
			results, err := iter.Run(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error but got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Run() error = %v, want substring %v", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error = %v", err)
			}

			if len(results) != len(tt.wantResults) {
				t.Fatalf("Run() results count = %v, want %v", len(results), len(tt.wantResults))
			}

			for i, result := range results {
				if result.Value != tt.wantResults[i] {
					t.Errorf("Run() result[%d].Value = %v, want %v", i, result.Value, tt.wantResults[i])
				}
				hasError := result.Error != nil
				if hasError != tt.wantErrors[i] {
					t.Errorf("Run() result[%d] has error = %v, want %v", i, hasError, tt.wantErrors[i])
				}
			}
		})
	}
}

// TestIteration_RunConcurrent tests concurrent processing behavior.
// Type: Iteration[int, int] - validates parallel execution with goroutines
func TestIteration_RunConcurrent(t *testing.T) {
	tests := []struct {
		name        string
		config      IterationConfig[int, int]
		input       []int
		wantResults []int
		wantErrors  []bool
		wantErr     bool
		errSubstr   string
	}{
		{
			name: "concurrent doubling",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: 3,
			},
			input:       []int{1, 2, 3, 4, 5},
			wantResults: []int{2, 4, 6, 8, 10},
			wantErrors:  []bool{false, false, false, false, false},
			wantErr:     false,
		},
		{
			name: "concurrent with delay",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					time.Sleep(10 * time.Millisecond)
					return val * 2, nil
				},
				ConcurrencyLimit: 5,
			},
			input:       []int{1, 2, 3, 4, 5},
			wantResults: []int{2, 4, 6, 8, 10},
			wantErrors:  []bool{false, false, false, false, false},
			wantErr:     false,
		},
		{
			name: "concurrent error stops all",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					if val == 3 {
						return 0, errors.New("value is 3")
					}
					return val * 2, nil
				},
				ContinueOnError:  false,
				ConcurrencyLimit: 3,
			},
			input:     []int{1, 2, 3, 4, 5},
			wantErr:   true,
			errSubstr: "iteration failed at index",
		},
		{
			name: "concurrent continue on error",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					if val%2 == 0 {
						return 0, errors.New("even value")
					}
					return val * 2, nil
				},
				ContinueOnError:  true,
				ConcurrencyLimit: 3,
			},
			input:       []int{1, 2, 3, 4, 5},
			wantResults: []int{2, 0, 6, 0, 10},
			wantErrors:  []bool{false, true, false, true, false},
			wantErr:     false,
		},
		{
			name: "unlimited concurrency",
			config: IterationConfig[int, int]{
				Processor: func(ctx context.Context, i int, val int) (int, error) {
					return val * 2, nil
				},
				ConcurrencyLimit: -1,
			},
			input:       []int{1, 2, 3, 4, 5},
			wantResults: []int{2, 4, 6, 8, 10},
			wantErrors:  []bool{false, false, false, false, false},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := NewIteration(tt.config)
			if err != nil {
				t.Fatalf("NewIteration() error = %v", err)
			}

			ctx := context.Background()
			results, err := iter.Run(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error but got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Run() error = %v, want substring %v", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error = %v", err)
			}

			if len(results) != len(tt.wantResults) {
				t.Fatalf("Run() results count = %v, want %v", len(results), len(tt.wantResults))
			}

			for i, result := range results {
				if result.Value != tt.wantResults[i] {
					t.Errorf("Run() result[%d].Value = %v, want %v", i, result.Value, tt.wantResults[i])
				}
				hasError := result.Error != nil
				if hasError != tt.wantErrors[i] {
					t.Errorf("Run() result[%d] has error = %v, want %v", i, hasError, tt.wantErrors[i])
				}
			}
		})
	}
}

// TestIteration_ConcurrencyVerification verifies actual concurrent execution.
// Type: Iteration[int, int] - validates goroutine concurrency limits using atomic counters
func TestIteration_ConcurrencyVerification(t *testing.T) {
	t.Run("verify concurrent execution", func(t *testing.T) {
		// Type: atomic counters tracking concurrent goroutines
		var activeCount int32
		var maxConcurrent int32

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				active := atomic.AddInt32(&activeCount, 1)

				// Track maximum concurrent executions
				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if active <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, active) {
						break
					}
				}

				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&activeCount, -1)
				return val, nil
			},
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		_, err = iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		maxReached := atomic.LoadInt32(&maxConcurrent)
		if maxReached != 3 {
			t.Errorf("maximum concurrent executions = %v, want 3", maxReached)
		}
	})

	t.Run("sequential execution - no concurrency", func(t *testing.T) {
		// Type: atomic counters verifying sequential execution (max=1)
		var activeCount int32
		var maxConcurrent int32

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				active := atomic.AddInt32(&activeCount, 1)

				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if active <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, active) {
						break
					}
				}

				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&activeCount, -1)
				return val, nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		_, err = iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		maxReached := atomic.LoadInt32(&maxConcurrent)
		if maxReached != 1 {
			t.Errorf("maximum concurrent executions = %v, want 1", maxReached)
		}
	})
}

// TestIteration_RunWithContext tests context handling in iteration execution.
// Type: Iteration[int, int] - validates context propagation and cancellation
func TestIteration_RunWithContext(t *testing.T) {
	t.Run("context cancellation in sequential", func(t *testing.T) {
		// Type: Iteration[int, int] with context cancellation during processing
		ctx, cancel := context.WithCancel(context.Background())

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				if i == 2 {
					cancel()
				}
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				default:
					return val * 2, nil
				}
			},
			ContinueOnError:  false,
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		_, err = iter.Run(ctx, []int{1, 2, 3, 4, 5})
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
	})

	t.Run("context cancellation in concurrent", func(t *testing.T) {
		// Type: Iteration[int, int] with timeout during concurrent processing
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				select {
				case <-time.After(100 * time.Millisecond):
					return val * 2, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			},
			ContinueOnError:  false,
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		_, err = iter.Run(ctx, []int{1, 2, 3, 4, 5})
		if err == nil {
			t.Errorf("Run() expected timeout error")
		}
	})

	t.Run("context value access", func(t *testing.T) {
		// Type: Iteration[int, int] with context value propagation
		type contextKey string
		key := contextKey("multiplier")

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				mult := ctx.Value(key).(int)
				return val * mult, nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		ctx := context.WithValue(context.Background(), key, 3)
		results, err := iter.Run(ctx, []int{1, 2, 3})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []int{3, 6, 9}
		for i, result := range results {
			if result.Value != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected[i])
			}
		}
	})
}

// TestIterationBuilder tests the builder pattern for iteration construction.
// Type: IterationBuilder[any, any] - validates fluent API for dynamic typing
func TestIterationBuilder(t *testing.T) {
	t.Run("complete builder chain", func(t *testing.T) {
		// Type: IterationBuilder[any, any] -> Iteration[any, any]
		iter, err := NewIterationBuilder[any, any]().
			WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
				return input.(int) * 2, nil
			}).
			WithConcurrencyLimit(3).
			WithContinueOnError(false).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		results, err := iter.Run(ctx, []any{1, 2, 3, 4, 5})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []int{2, 4, 6, 8, 10}
		for i, result := range results {
			if result.Value.(int) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected[i])
			}
		}
	})

	t.Run("builder without processor", func(t *testing.T) {
		// Type: IterationBuilder[any, any] - missing processor validation
		_, err := NewIterationBuilder[any, any]().
			WithConcurrencyLimit(3).
			Build()

		if err == nil {
			t.Errorf("Build() expected error for missing processor")
		}
	})

	t.Run("builder with type conversion", func(t *testing.T) {
		// Type: IterationBuilder[any, any] with int -> string conversion
		iter, err := NewIterationBuilder[any, any]().
			WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
				return strconv.Itoa(input.(int)), nil
			}).
			WithConcurrencyLimit(1).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		results, err := iter.Run(ctx, []any{1, 2, 3})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []string{"1", "2", "3"}
		for i, result := range results {
			if result.Value.(string) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected[i])
			}
		}
	})
}

// TestIteration_ComplexScenarios tests real-world usage patterns.
func TestIteration_ComplexScenarios(t *testing.T) {
	t.Run("data validation pipeline", func(t *testing.T) {
		// Type: Iteration[User, User] - domain model validation and transformation
		type User struct {
			ID   int
			Name string
		}

		config := IterationConfig[User, User]{
			Processor: func(ctx context.Context, i int, user User) (User, error) {
				if user.Name == "" {
					return User{}, fmt.Errorf("user %d has empty name", user.ID)
				}
				user.Name = strings.ToUpper(user.Name)
				return user, nil
			},
			ContinueOnError:  true,
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []User{
			{ID: 1, Name: "alice"},
			{ID: 2, Name: ""},
			{ID: 3, Name: "bob"},
		}

		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Error != nil {
			t.Errorf("result[0] should not have error")
		}
		if results[0].Value.Name != "ALICE" {
			t.Errorf("result[0].Name = %v, want ALICE", results[0].Value.Name)
		}

		if results[1].Error == nil {
			t.Errorf("result[1] should have error")
		}

		if results[2].Error != nil {
			t.Errorf("result[2] should not have error")
		}
		if results[2].Value.Name != "BOB" {
			t.Errorf("result[2].Name = %v, want BOB", results[2].Value.Name)
		}
	})

	t.Run("concurrent API calls simulation", func(t *testing.T) {
		// Type: Iteration[int, string] - simulates parallel HTTP requests
		var callCount int32
		var mu sync.Mutex
		callOrder := []int{}

		config := IterationConfig[int, string]{
			Processor: func(ctx context.Context, i int, id int) (string, error) {
				atomic.AddInt32(&callCount, 1)

				mu.Lock()
				callOrder = append(callOrder, id)
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)
				return fmt.Sprintf("data_%d", id), nil
			},
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if atomic.LoadInt32(&callCount) != 5 {
			t.Errorf("call count = %v, want 5", callCount)
		}

		for i, result := range results {
			expected := fmt.Sprintf("data_%d", input[i])
			if result.Value != expected {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected)
			}
		}
	})

	t.Run("batch processing with retry logic", func(t *testing.T) {
		// Type: Iteration[int, int] - failure simulation with retry tracking
		failedAttempts := make(map[int]int)
		var mu sync.Mutex

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				mu.Lock()
				attempts := failedAttempts[i]
				failedAttempts[i]++
				mu.Unlock()

				// Fail first attempt for even indices
				if i%2 == 0 && attempts == 0 {
					return 0, fmt.Errorf("temporary failure at index %d", i)
				}

				return val * 2, nil
			},
			ContinueOnError:  true,
			ConcurrencyLimit: 2,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Check that even indices have errors
		for i, result := range results {
			if i%2 == 0 {
				if result.Error == nil {
					t.Errorf("result[%d] should have error", i)
				}
			} else {
				if result.Error != nil {
					t.Errorf("result[%d] should not have error: %v", i, result.Error)
				}
				if result.Value != input[i]*2 {
					t.Errorf("result[%d] = %v, want %v", i, result.Value, input[i]*2)
				}
			}
		}
	})

	t.Run("filter and transform", func(t *testing.T) {
		// Type: Iteration[int, string] - filtering with error propagation
		config := IterationConfig[int, string]{
			Processor: func(ctx context.Context, i int, val int) (string, error) {
				if val%2 == 0 {
					return "", fmt.Errorf("skip even number: %d", val)
				}
				return fmt.Sprintf("odd_%d", val), nil
			},
			ContinueOnError:  true,
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5, 6, 7}
		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Collect successful results
		var oddNumbers []string
		for _, result := range results {
			if result.Error == nil {
				oddNumbers = append(oddNumbers, result.Value)
			}
		}

		expected := []string{"odd_1", "odd_3", "odd_5", "odd_7"}
		if len(oddNumbers) != len(expected) {
			t.Errorf("filtered results count = %v, want %v", len(oddNumbers), len(expected))
		}

		for i, val := range oddNumbers {
			if val != expected[i] {
				t.Errorf("oddNumbers[%d] = %v, want %v", i, val, expected[i])
			}
		}
	})

	t.Run("aggregation with concurrent processing", func(t *testing.T) {
		// Type: Iteration[int, int] with shared state aggregation using mutex
		type Stats struct {
			Sum   int
			Count int
		}

		var mu sync.Mutex
		globalStats := Stats{}

		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				squared := val * val

				mu.Lock()
				globalStats.Sum += squared
				globalStats.Count++
				mu.Unlock()

				return squared, nil
			},
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expectedSquares := []int{1, 4, 9, 16, 25}
		for i, result := range results {
			if result.Value != expectedSquares[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expectedSquares[i])
			}
		}

		if globalStats.Count != 5 {
			t.Errorf("globalStats.Count = %v, want 5", globalStats.Count)
		}
		if globalStats.Sum != 55 { // 1+4+9+16+25
			t.Errorf("globalStats.Sum = %v, want 55", globalStats.Sum)
		}
	})
}

// TestIteration_EdgeCases tests edge cases and boundary conditions.
// Type: Various Iteration[T, U] configurations - validates robustness
func TestIteration_EdgeCases(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		// Type: Iteration[int, int] with empty input
		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				return val * 2, nil
			},
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		results, err := iter.Run(context.Background(), []int{})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if len(results) != 0 {
			t.Errorf("results length = %v, want 0", len(results))
		}
	})

	t.Run("single element", func(t *testing.T) {
		// Type: Iteration[int, int] with single item processing
		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				return val * 2, nil
			},
			ConcurrencyLimit: 3,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		results, err := iter.Run(context.Background(), []int{5})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if len(results) != 1 {
			t.Errorf("results length = %v, want 1", len(results))
		}
		if results[0].Value != 10 {
			t.Errorf("result = %v, want 10", results[0].Value)
		}
	})

	t.Run("large input", func(t *testing.T) {
		// Type: Iteration[int, int] with 1000 elements - scalability test
		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				return val * 2, nil
			},
			ConcurrencyLimit: 10,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := make([]int, 1000)
		for i := range input {
			input[i] = i
		}

		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if len(results) != 1000 {
			t.Errorf("results length = %v, want 1000", len(results))
		}

		for i, result := range results {
			if result.Value != i*2 {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, i*2)
			}
		}
	})

	t.Run("all processors fail", func(t *testing.T) {
		// Type: Iteration[int, int] with universal failure handling
		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				return 0, errors.New("always fails")
			},
			ContinueOnError:  true,
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		results, err := iter.Run(context.Background(), []int{1, 2, 3})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		for i, result := range results {
			if result.Error == nil {
				t.Errorf("result[%d] should have error", i)
			}
		}
	})

	t.Run("zero value results", func(t *testing.T) {
		// Type: Iteration[int, int] with zero value outputs
		config := IterationConfig[int, int]{
			Processor: func(ctx context.Context, i int, val int) (int, error) {
				return 0, nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		results, err := iter.Run(context.Background(), []int{1, 2, 3})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		for i, result := range results {
			if result.Value != 0 {
				t.Errorf("result[%d] = %v, want 0", i, result.Value)
			}
			if result.Error != nil {
				t.Errorf("result[%d] should not have error", i)
			}
		}
	})
}

// TestIteration_DifferentTypes tests various type combinations.
func TestIteration_DifferentTypes(t *testing.T) {
	t.Run("string to int", func(t *testing.T) {
		// Type: Iteration[string, int] - string length calculation
		config := IterationConfig[string, int]{
			Processor: func(ctx context.Context, i int, val string) (int, error) {
				return len(val), nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []string{"a", "bb", "ccc"}
		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []int{1, 2, 3}
		for i, result := range results {
			if result.Value != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected[i])
			}
		}
	})

	t.Run("struct transformation", func(t *testing.T) {
		// Type: Iteration[Input, Output] - domain model transformation
		type Input struct {
			ID   int
			Name string
		}
		type Output struct {
			FullName string
			Length   int
		}

		config := IterationConfig[Input, Output]{
			Processor: func(ctx context.Context, i int, val Input) (Output, error) {
				fullName := fmt.Sprintf("User_%s_%d", val.Name, val.ID)
				return Output{
					FullName: fullName,
					Length:   len(fullName),
				}, nil
			},
			ConcurrencyLimit: 2,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []Input{
			{ID: 1, Name: "Alice"},
			{ID: 2, Name: "Bob"},
		}

		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value.FullName != "User_Alice_1" {
			t.Errorf("result[0].FullName = %v, want User_Alice_1", results[0].Value.FullName)
		}
		if results[1].Value.FullName != "User_Bob_2" {
			t.Errorf("result[1].FullName = %v, want User_Bob_2", results[1].Value.FullName)
		}
	})

	t.Run("slice to slice element", func(t *testing.T) {
		// Type: Iteration[[]int, int] - slice reduction/aggregation
		config := IterationConfig[[]int, int]{
			Processor: func(ctx context.Context, i int, val []int) (int, error) {
				sum := 0
				for _, v := range val {
					sum += v
				}
				return sum, nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := [][]int{
			{1, 2, 3},
			{4, 5},
			{6},
		}

		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []int{6, 9, 6}
		for i, result := range results {
			if result.Value != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected[i])
			}
		}
	})

	t.Run("pointer types", func(t *testing.T) {
		// Type: Iteration[*Data, *Data] - pointer-based transformation
		type Data struct {
			Value int
		}

		config := IterationConfig[*Data, *Data]{
			Processor: func(ctx context.Context, i int, val *Data) (*Data, error) {
				return &Data{Value: val.Value * 2}, nil
			},
			ConcurrencyLimit: 1,
		}

		iter, err := NewIteration(config)
		if err != nil {
			t.Fatalf("NewIteration() error = %v", err)
		}

		input := []*Data{
			{Value: 1},
			{Value: 2},
			{Value: 3},
		}

		results, err := iter.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		expected := []int{2, 4, 6}
		for i, result := range results {
			if result.Value.Value != expected[i] {
				t.Errorf("result[%d].Value = %v, want %v", i, result.Value.Value, expected[i])
			}
		}
	})
}

// TestIteration_SequentialVsConcurrent compares sequential and concurrent performance.
// Type: Iteration[int, int] - performance benchmarking with different concurrency levels
func TestIteration_SequentialVsConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance comparison in short mode")
	}

	input := make([]int, 100)
	for i := range input {
		input[i] = i
	}

	processor := func(ctx context.Context, i int, val int) (int, error) {
		time.Sleep(5 * time.Millisecond) // Simulate work
		return val * 2, nil
	}

	t.Run("sequential", func(t *testing.T) {
		// Type: Iteration[int, int] with sequential execution (concurrency=1)
		config := IterationConfig[int, int]{
			Processor:        processor,
			ConcurrencyLimit: 1,
		}

		iter, _ := NewIteration(config)
		start := time.Now()
		_, err := iter.Run(context.Background(), input)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Sequential processing took: %v", duration)
	})

	t.Run("concurrent with limit 10", func(t *testing.T) {
		// Type: Iteration[int, int] with concurrent execution (concurrency=10)
		config := IterationConfig[int, int]{
			Processor:        processor,
			ConcurrencyLimit: 10,
		}

		iter, _ := NewIteration(config)
		start := time.Now()
		_, err := iter.Run(context.Background(), input)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Concurrent processing (limit=10) took: %v", duration)
	})
}
