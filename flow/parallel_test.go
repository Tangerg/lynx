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

// TestParallelConfig_validate tests the validation logic for ParallelConfig.
// Type: ParallelConfig[int, int] - validates configuration rules for parallel execution
func TestParallelConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ParallelConfig[int, int]
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "parallel config cannot be nil",
		},
		{
			name: "empty processors",
			config: &ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){},
			},
			wantErr: true,
			errMsg:  "at least one processor is required",
		},
		{
			name: "nil processors slice",
			config: &ParallelConfig[int, int]{
				Processors: nil,
			},
			wantErr: true,
			errMsg:  "at least one processor is required",
		},
		{
			name: "valid single processor",
			config: &ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid multiple processors",
			config: &ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i + 10, nil
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with concurrency limit",
			config: &ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
				},
				ConcurrencyLimit: 5,
			},
			wantErr: false,
		},
		{
			name: "valid with continueOnError",
			config: &ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
				},
				ContinueOnError: true,
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
			}
		})
	}
}

// TestNewParallel tests the constructor for Parallel.
// Type: Parallel[int, int] - verifies proper initialization of parallel processor
func TestNewParallel(t *testing.T) {
	tests := []struct {
		name    string
		config  ParallelConfig[int, int]
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty processors",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parallel, err := NewParallel(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewParallel() expected error but got nil")
				}
				if parallel != nil {
					t.Errorf("NewParallel() expected nil parallel on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewParallel() unexpected error = %v", err)
				}
				if parallel == nil {
					t.Errorf("NewParallel() returned nil parallel")
				}
			}
		})
	}
}

// TestParallel_calcConcurrencyLimit tests concurrency limit calculation.
// Type: Parallel[int, int] - validates dynamic concurrency adjustment based on processor count
func TestParallel_calcConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name             string
		concurrencyLimit int
		numProcessors    int
		want             int
	}{
		{
			name:             "unlimited (0)",
			concurrencyLimit: 0,
			numProcessors:    5,
			want:             5,
		},
		{
			name:             "unlimited (negative)",
			concurrencyLimit: -1,
			numProcessors:    5,
			want:             5,
		},
		{
			name:             "limit less than processors",
			concurrencyLimit: 3,
			numProcessors:    5,
			want:             3,
		},
		{
			name:             "limit more than processors",
			concurrencyLimit: 10,
			numProcessors:    5,
			want:             5,
		},
		{
			name:             "limit equal to processors",
			concurrencyLimit: 5,
			numProcessors:    5,
			want:             5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Type: []func(context.Context, int) (int, error) - slice of processors
			processors := make([]func(context.Context, int) (int, error), tt.numProcessors)
			for i := range processors {
				processors[i] = func(ctx context.Context, input int) (int, error) {
					return input, nil
				}
			}

			p := &Parallel[int, int]{
				processors:       processors,
				concurrencyLimit: tt.concurrencyLimit,
			}

			got := p.calcConcurrencyLimit()
			if got != tt.want {
				t.Errorf("calcConcurrencyLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParallel_Run tests basic parallel execution.
// Type: Parallel[int, int] - validates concurrent execution of multiple processors on same input
func TestParallel_Run(t *testing.T) {
	tests := []struct {
		name        string
		config      ParallelConfig[int, int]
		input       int
		wantResults []int
		wantErrors  []bool
		wantErr     bool
		errSubstr   string
	}{
		{
			name: "single processor",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
				},
			},
			input:       5,
			wantResults: []int{10},
			wantErrors:  []bool{false},
			wantErr:     false,
		},
		{
			name: "multiple processors - same input",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i + 10, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i * i, nil
					},
				},
			},
			input:       5,
			wantResults: []int{10, 15, 25}, // 5*2, 5+10, 5*5
			wantErrors:  []bool{false, false, false},
			wantErr:     false,
		},
		{
			name: "error stops all processors",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return 0, errors.New("processor error")
					},
					func(ctx context.Context, i int) (int, error) {
						return i + 10, nil
					},
				},
				ContinueOnError: false,
			},
			input:     5,
			wantErr:   true,
			errSubstr: "processor 1 failed",
		},
		{
			name: "continue on error",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return 0, errors.New("processor error")
					},
					func(ctx context.Context, i int) (int, error) {
						return i + 10, nil
					},
				},
				ContinueOnError: true,
			},
			input:       5,
			wantResults: []int{10, 0, 15},
			wantErrors:  []bool{false, true, false},
			wantErr:     false,
		},
		{
			name: "all processors succeed",
			config: ParallelConfig[int, int]{
				Processors: []func(context.Context, int) (int, error){
					func(ctx context.Context, i int) (int, error) {
						return i * 1, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i * 2, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i * 3, nil
					},
					func(ctx context.Context, i int) (int, error) {
						return i * 4, nil
					},
				},
			},
			input:       3,
			wantResults: []int{3, 6, 9, 12},
			wantErrors:  []bool{false, false, false, false},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parallel, err := NewParallel(tt.config)
			if err != nil {
				t.Fatalf("NewParallel() error = %v", err)
			}

			ctx := context.Background()
			// Type: []Result[int] - slice of results from parallel execution
			results, err := parallel.Run(ctx, tt.input)

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

// TestParallel_ConcurrencyVerification verifies actual concurrent execution.
// Type: Parallel[int, int] - validates goroutine concurrency limits using atomic counters
func TestParallel_ConcurrencyVerification(t *testing.T) {
	t.Run("verify concurrent execution", func(t *testing.T) {
		// Type: atomic counters tracking concurrent goroutines
		var activeCount int32
		var maxConcurrent int32

		// Type: []func(context.Context, int) (int, error) - slice of 5 processors
		processors := make([]func(context.Context, int) (int, error), 5)
		for i := range processors {
			idx := i
			processors[i] = func(ctx context.Context, input int) (int, error) {
				active := atomic.AddInt32(&activeCount, 1)

				// Track maximum concurrent executions
				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if active <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, active) {
						break
					}
				}

				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&activeCount, -1)
				return input * idx, nil
			}
		}

		config := ParallelConfig[int, int]{
			Processors:       processors,
			ConcurrencyLimit: 0, // Unlimited
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		_, err = parallel.Run(context.Background(), 10)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		maxReached := atomic.LoadInt32(&maxConcurrent)
		if maxReached != 5 {
			t.Errorf("maximum concurrent executions = %v, want 5", maxReached)
		}
	})

	t.Run("verify concurrency limit", func(t *testing.T) {
		// Type: atomic counters verifying max 3 concurrent goroutines
		var activeCount int32
		var maxConcurrent int32

		// Type: []func(context.Context, int) (int, error) - 10 processors limited to 3 concurrent
		processors := make([]func(context.Context, int) (int, error), 10)
		for i := range processors {
			idx := i
			processors[i] = func(ctx context.Context, input int) (int, error) {
				active := atomic.AddInt32(&activeCount, 1)

				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if active <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, active) {
						break
					}
				}

				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&activeCount, -1)
				return input * idx, nil
			}
		}

		config := ParallelConfig[int, int]{
			Processors:       processors,
			ConcurrencyLimit: 3,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		_, err = parallel.Run(context.Background(), 10)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		maxReached := atomic.LoadInt32(&maxConcurrent)
		if maxReached != 3 {
			t.Errorf("maximum concurrent executions = %v, want 3", maxReached)
		}
	})
}

// TestParallel_RunWithContext tests context handling in parallel execution.
// Type: Parallel[int, int] - validates context propagation and cancellation
func TestParallel_RunWithContext(t *testing.T) {
	t.Run("context cancellation stops all", func(t *testing.T) {
		// Type: Parallel[int, int] with mid-execution cancellation
		ctx, cancel := context.WithCancel(context.Background())

		var started int32

		// Type: []func(context.Context, int) (int, error) - processors with cancellation
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&started, 1)
				time.Sleep(10 * time.Millisecond)
				return i * 2, nil
			},
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&started, 1)
				cancel() // Cancel here
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return i + 10, nil
				}
			},
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&started, 1)
				time.Sleep(10 * time.Millisecond)
				return i * 3, nil
			},
		}

		config := ParallelConfig[int, int]{
			Processors:      processors,
			ContinueOnError: false,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		_, err = parallel.Run(ctx, 5)
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		// Type: Parallel[int, int] with timeout during execution
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		// Type: []func(context.Context, int) (int, error) - slow processors exceeding timeout
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				select {
				case <-time.After(100 * time.Millisecond):
					return i * 2, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			},
			func(ctx context.Context, i int) (int, error) {
				select {
				case <-time.After(100 * time.Millisecond):
					return i + 10, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			},
		}

		config := ParallelConfig[int, int]{
			Processors:      processors,
			ContinueOnError: false,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		_, err = parallel.Run(ctx, 5)
		if err == nil {
			t.Errorf("Run() expected timeout error")
		}
	})

	t.Run("context value access", func(t *testing.T) {
		// Type: Parallel[int, int] with context value propagation
		type contextKey string
		key := contextKey("multiplier")

		// Type: []func(context.Context, int) (int, error) - processors accessing context values
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				mult := ctx.Value(key).(int)
				return i * mult, nil
			},
			func(ctx context.Context, i int) (int, error) {
				mult := ctx.Value(key).(int)
				return i + mult, nil
			},
		}

		config := ParallelConfig[int, int]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		ctx := context.WithValue(context.Background(), key, 10)
		results, err := parallel.Run(ctx, 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value != 50 { // 5 * 10
			t.Errorf("result[0] = %v, want 50", results[0].Value)
		}
		if results[1].Value != 15 { // 5 + 10
			t.Errorf("result[1] = %v, want 15", results[1].Value)
		}
	})
}

// TestParallelBuilder tests the builder pattern for parallel construction.
// Type: ParallelBuilder[any, any] - validates fluent API for dynamic typing
func TestParallelBuilder(t *testing.T) {
	t.Run("complete builder chain", func(t *testing.T) {
		// Type: []func(context.Context, any) (any, error) - dynamic processors
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			},
			func(ctx context.Context, input any) (any, error) {
				return input.(int) + 10, nil
			},
		}

		// Type: ParallelBuilder[any, any] -> Parallel[any, any]
		parallel, err := NewParallelBuilder[any, any]().
			WithProcessors(processors).
			WithConcurrencyLimit(2).
			WithContinueOnError(false).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		results, err := parallel.Run(ctx, 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("results count = %v, want 2", len(results))
		}

		if results[0].Value.(int) != 10 {
			t.Errorf("result[0] = %v, want 10", results[0].Value)
		}
		if results[1].Value.(int) != 15 {
			t.Errorf("result[1] = %v, want 15", results[1].Value)
		}
	})

	t.Run("builder without processors", func(t *testing.T) {
		// Type: ParallelBuilder[any, any] - missing processors validation
		_, err := NewParallelBuilder[any, any]().
			WithConcurrencyLimit(2).
			Build()

		if err == nil {
			t.Errorf("Build() expected error for missing processors")
		}
	})

	t.Run("builder with type conversion", func(t *testing.T) {
		// Type: []func(context.Context, any) (any, error) with int -> string conversion
		processors := []func(context.Context, any) (any, error){
			func(ctx context.Context, input any) (any, error) {
				return strconv.Itoa(input.(int)), nil
			},
			func(ctx context.Context, input any) (any, error) {
				return fmt.Sprintf("value_%d", input.(int)), nil
			},
		}

		parallel, err := NewParallelBuilder[any, any]().
			WithProcessors(processors).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		results, err := parallel.Run(ctx, 42)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value.(string) != "42" {
			t.Errorf("result[0] = %v, want '42'", results[0].Value)
		}
		if results[1].Value.(string) != "value_42" {
			t.Errorf("result[1] = %v, want 'value_42'", results[1].Value)
		}
	})
}

// TestParallel_ComplexScenarios tests real-world usage patterns.
func TestParallel_ComplexScenarios(t *testing.T) {
	t.Run("multiple API calls with same input", func(t *testing.T) {
		// Type: Parallel[string, APIResponse] - simulates parallel API calls
		type APIResponse struct {
			Source string
			Data   string
		}

		// Type: []func(context.Context, string) (APIResponse, error) - API simulators
		processors := []func(context.Context, string) (APIResponse, error){
			func(ctx context.Context, query string) (APIResponse, error) {
				time.Sleep(10 * time.Millisecond)
				return APIResponse{Source: "API1", Data: fmt.Sprintf("api1_%s", query)}, nil
			},
			func(ctx context.Context, query string) (APIResponse, error) {
				time.Sleep(15 * time.Millisecond)
				return APIResponse{Source: "API2", Data: fmt.Sprintf("api2_%s", query)}, nil
			},
			func(ctx context.Context, query string) (APIResponse, error) {
				time.Sleep(5 * time.Millisecond)
				return APIResponse{Source: "API3", Data: fmt.Sprintf("api3_%s", query)}, nil
			},
		}

		config := ParallelConfig[string, APIResponse]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		start := time.Now()
		results, err := parallel.Run(context.Background(), "test_query")
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Should complete in ~15ms (max duration) not 30ms (sum)
		if duration > 50*time.Millisecond {
			t.Errorf("parallel execution took too long: %v", duration)
		}

		if len(results) != 3 {
			t.Fatalf("results count = %v, want 3", len(results))
		}

		sources := []string{"API1", "API2", "API3"}
		for i, result := range results {
			if result.Error != nil {
				t.Errorf("result[%d] has error: %v", i, result.Error)
			}
			if result.Value.Source != sources[i] {
				t.Errorf("result[%d].Source = %v, want %v", i, result.Value.Source, sources[i])
			}
		}
	})

	t.Run("data enrichment from multiple sources", func(t *testing.T) {
		// Type: Parallel[int, string] - parallel data enrichment pattern
		type UserData struct {
			ID      int
			Profile string
			Orders  string
			Reviews string
		}

		// Type: []func(context.Context, int) (string, error) - data source processors
		processors := []func(context.Context, int) (string, error){
			// Get profile
			func(ctx context.Context, userID int) (string, error) {
				return fmt.Sprintf("profile_%d", userID), nil
			},
			// Get orders
			func(ctx context.Context, userID int) (string, error) {
				return fmt.Sprintf("orders_%d", userID), nil
			},
			// Get reviews
			func(ctx context.Context, userID int) (string, error) {
				return fmt.Sprintf("reviews_%d", userID), nil
			},
		}

		config := ParallelConfig[int, string]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 123)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// Type: UserData - aggregated result from parallel processors
		userData := UserData{
			ID:      123,
			Profile: results[0].Value,
			Orders:  results[1].Value,
			Reviews: results[2].Value,
		}

		if userData.Profile != "profile_123" {
			t.Errorf("userData.Profile = %v, want 'profile_123'", userData.Profile)
		}
		if userData.Orders != "orders_123" {
			t.Errorf("userData.Orders = %v, want 'orders_123'", userData.Orders)
		}
		if userData.Reviews != "reviews_123" {
			t.Errorf("userData.Reviews = %v, want 'reviews_123'", userData.Reviews)
		}
	})

	t.Run("partial failure with continue on error", func(t *testing.T) {
		// Type: Parallel[int, int] with partial failure handling
		var callCount int32

		// Type: []func(context.Context, int) (int, error) - processors with one failing
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&callCount, 1)
				return i * 2, nil
			},
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&callCount, 1)
				return 0, errors.New("service unavailable")
			},
			func(ctx context.Context, i int) (int, error) {
				atomic.AddInt32(&callCount, 1)
				return i + 10, nil
			},
		}

		config := ParallelConfig[int, int]{
			Processors:      processors,
			ContinueOnError: true,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// All processors should have been called
		if atomic.LoadInt32(&callCount) != 3 {
			t.Errorf("call count = %v, want 3", callCount)
		}

		// Check individual results
		if results[0].Error != nil {
			t.Errorf("result[0] should not have error")
		}
		if results[1].Error == nil {
			t.Errorf("result[1] should have error")
		}
		if results[2].Error != nil {
			t.Errorf("result[2] should not have error")
		}

		if results[0].Value != 10 {
			t.Errorf("result[0] = %v, want 10", results[0].Value)
		}
		if results[2].Value != 15 {
			t.Errorf("result[2] = %v, want 15", results[2].Value)
		}
	})

	t.Run("aggregation pattern", func(t *testing.T) {
		// Type: Parallel[[]int, int] - parallel statistical aggregation
		// Type: []func(context.Context, []int) (int, error) - statistical processors
		processors := []func(context.Context, []int) (int, error){
			// Sum
			func(ctx context.Context, nums []int) (int, error) {
				sum := 0
				for _, n := range nums {
					sum += n
				}
				return sum, nil
			},
			// Max
			func(ctx context.Context, nums []int) (int, error) {
				if len(nums) == 0 {
					return 0, nil
				}
				max := nums[0]
				for _, n := range nums {
					if n > max {
						max = n
					}
				}
				return max, nil
			},
			// Min
			func(ctx context.Context, nums []int) (int, error) {
				if len(nums) == 0 {
					return 0, nil
				}
				min := nums[0]
				for _, n := range nums {
					if n < min {
						min = n
					}
				}
				return min, nil
			},
			// Count
			func(ctx context.Context, nums []int) (int, error) {
				return len(nums), nil
			},
		}

		config := ParallelConfig[[]int, int]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		input := []int{1, 5, 3, 9, 2, 7}
		results, err := parallel.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value != 27 { // sum
			t.Errorf("sum = %v, want 27", results[0].Value)
		}
		if results[1].Value != 9 { // max
			t.Errorf("max = %v, want 9", results[1].Value)
		}
		if results[2].Value != 1 { // min
			t.Errorf("min = %v, want 1", results[2].Value)
		}
		if results[3].Value != 6 { // count
			t.Errorf("count = %v, want 6", results[3].Value)
		}
	})

	t.Run("validation from multiple validators", func(t *testing.T) {
		// Type: Parallel[string, ValidationResult] - parallel validation pipeline
		type ValidationResult struct {
			Valid   bool
			Message string
		}

		// Type: []func(context.Context, string) (ValidationResult, error) - validator processors
		processors := []func(context.Context, string) (ValidationResult, error){
			// Length validator
			func(ctx context.Context, input string) (ValidationResult, error) {
				if len(input) < 3 {
					return ValidationResult{Valid: false, Message: "too short"}, nil
				}
				return ValidationResult{Valid: true, Message: "length ok"}, nil
			},
			// Character validator
			func(ctx context.Context, input string) (ValidationResult, error) {
				for _, c := range input {
					if c < 'a' || c > 'z' {
						return ValidationResult{Valid: false, Message: "invalid characters"}, nil
					}
				}
				return ValidationResult{Valid: true, Message: "characters ok"}, nil
			},
			// Blacklist validator
			func(ctx context.Context, input string) (ValidationResult, error) {
				blacklist := []string{"bad", "evil", "wrong"}
				for _, word := range blacklist {
					if input == word {
						return ValidationResult{Valid: false, Message: "blacklisted"}, nil
					}
				}
				return ValidationResult{Valid: true, Message: "not blacklisted"}, nil
			},
		}

		config := ParallelConfig[string, ValidationResult]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		// Test valid input
		results, err := parallel.Run(context.Background(), "hello")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		allValid := true
		for _, result := range results {
			if !result.Value.Valid {
				allValid = false
				break
			}
		}
		if !allValid {
			t.Errorf("all validators should pass for 'hello'")
		}

		// Test invalid input
		results, err = parallel.Run(context.Background(), "bad")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[2].Value.Valid {
			t.Errorf("blacklist validator should fail for 'bad'")
		}
	})
}

// TestParallel_EdgeCases tests edge cases and boundary conditions.
// Type: Various Parallel[T, U] configurations - validates robustness
func TestParallel_EdgeCases(t *testing.T) {
	t.Run("single processor", func(t *testing.T) {
		// Type: Parallel[int, int] with single processor (no true parallelism)
		config := ParallelConfig[int, int]{
			Processors: []func(context.Context, int) (int, error){
				func(ctx context.Context, i int) (int, error) {
					return i * 2, nil
				},
			},
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("results count = %v, want 1", len(results))
		}
		if results[0].Value != 10 {
			t.Errorf("result = %v, want 10", results[0].Value)
		}
	})

	t.Run("all processors fail", func(t *testing.T) {
		// Type: Parallel[int, int] with universal failure handling
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				return 0, errors.New("error 1")
			},
			func(ctx context.Context, i int) (int, error) {
				return 0, errors.New("error 2")
			},
		}

		config := ParallelConfig[int, int]{
			Processors:      processors,
			ContinueOnError: true,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		for i, result := range results {
			if result.Error == nil {
				t.Errorf("result[%d] should have error", i)
			}
		}
	})

	t.Run("zero value input", func(t *testing.T) {
		// Type: Parallel[int, int] with zero value processing
		processors := []func(context.Context, int) (int, error){
			func(ctx context.Context, i int) (int, error) {
				return i * 2, nil
			},
			func(ctx context.Context, i int) (int, error) {
				return i + 10, nil
			},
		}

		config := ParallelConfig[int, int]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 0)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value != 0 {
			t.Errorf("result[0] = %v, want 0", results[0].Value)
		}
		if results[1].Value != 10 {
			t.Errorf("result[1] = %v, want 10", results[1].Value)
		}
	})

	t.Run("many processors", func(t *testing.T) {
		// Type: Parallel[int, int] with 100 processors - scalability test
		numProcessors := 100
		// Type: []func(context.Context, int) (int, error) - large processor slice
		processors := make([]func(context.Context, int) (int, error), numProcessors)
		for i := range processors {
			idx := i
			processors[i] = func(ctx context.Context, input int) (int, error) {
				return input + idx, nil
			}
		}

		config := ParallelConfig[int, int]{
			Processors:       processors,
			ConcurrencyLimit: 10,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 1)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if len(results) != numProcessors {
			t.Fatalf("results count = %v, want %v", len(results), numProcessors)
		}

		for i, result := range results {
			expected := 1 + i
			if result.Value != expected {
				t.Errorf("result[%d] = %v, want %v", i, result.Value, expected)
			}
		}
	})
}

// TestParallel_DifferentTypes tests various type combinations.
func TestParallel_DifferentTypes(t *testing.T) {
	t.Run("int to different outputs", func(t *testing.T) {
		// Type: Parallel[int, Output] - single input to multiple output formats
		type Output struct {
			Type  string
			Value string
		}

		// Type: []func(context.Context, int) (Output, error) - format converters
		processors := []func(context.Context, int) (Output, error){
			func(ctx context.Context, i int) (Output, error) {
				return Output{Type: "string", Value: strconv.Itoa(i)}, nil
			},
			func(ctx context.Context, i int) (Output, error) {
				return Output{Type: "hex", Value: fmt.Sprintf("0x%x", i)}, nil
			},
			func(ctx context.Context, i int) (Output, error) {
				return Output{Type: "binary", Value: fmt.Sprintf("%b", i)}, nil
			},
		}

		config := ParallelConfig[int, Output]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		results, err := parallel.Run(context.Background(), 42)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value.Value != "42" {
			t.Errorf("string conversion = %v, want '42'", results[0].Value.Value)
		}
		if results[1].Value.Value != "0x2a" {
			t.Errorf("hex conversion = %v, want '0x2a'", results[1].Value.Value)
		}
		if results[2].Value.Value != "101010" {
			t.Errorf("binary conversion = %v, want '101010'", results[2].Value.Value)
		}
	})

	t.Run("struct input to various outputs", func(t *testing.T) {
		// Type: Parallel[User, string] - struct transformation to multiple string formats
		type User struct {
			Name string
			Age  int
		}

		// Type: []func(context.Context, User) (string, error) - string transformers
		processors := []func(context.Context, User) (string, error){
			func(ctx context.Context, u User) (string, error) {
				return strings.ToUpper(u.Name), nil
			},
			func(ctx context.Context, u User) (string, error) {
				return strings.ToLower(u.Name), nil
			},
			func(ctx context.Context, u User) (string, error) {
				return fmt.Sprintf("%s_%d", u.Name, u.Age), nil
			},
		}

		config := ParallelConfig[User, string]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		user := User{Name: "Alice", Age: 30}
		results, err := parallel.Run(context.Background(), user)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if results[0].Value != "ALICE" {
			t.Errorf("uppercase = %v, want 'ALICE'", results[0].Value)
		}
		if results[1].Value != "alice" {
			t.Errorf("lowercase = %v, want 'alice'", results[1].Value)
		}
		if results[2].Value != "Alice_30" {
			t.Errorf("formatted = %v, want 'Alice_30'", results[2].Value)
		}
	})
}

// TestParallel_ProcessorIsolation verifies processors don't affect each other.
// Type: Parallel[MutableData, []int] - validates input isolation between concurrent processors
func TestParallel_ProcessorIsolation(t *testing.T) {
	t.Run("modifications to input don't affect other processors", func(t *testing.T) {
		// Type: MutableData - potentially mutable input structure
		type MutableData struct {
			Values []int
		}

		var mu sync.Mutex
		// Type: [][]int - collected results from each processor
		results := make([][]int, 3)

		// Type: []func(context.Context, MutableData) ([]int, error) - processors with timing
		processors := []func(context.Context, MutableData) ([]int, error){
			func(ctx context.Context, data MutableData) ([]int, error) {
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				results[0] = append([]int{}, data.Values...)
				mu.Unlock()
				return data.Values, nil
			},
			func(ctx context.Context, data MutableData) ([]int, error) {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				results[1] = append([]int{}, data.Values...)
				mu.Unlock()
				return data.Values, nil
			},
			func(ctx context.Context, data MutableData) ([]int, error) {
				time.Sleep(15 * time.Millisecond)
				mu.Lock()
				results[2] = append([]int{}, data.Values...)
				mu.Unlock()
				return data.Values, nil
			},
		}

		config := ParallelConfig[MutableData, []int]{
			Processors: processors,
		}

		parallel, err := NewParallel(config)
		if err != nil {
			t.Fatalf("NewParallel() error = %v", err)
		}

		input := MutableData{Values: []int{1, 2, 3}}
		_, err = parallel.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// All processors should see the same original input
		for i, result := range results {
			if len(result) != 3 {
				t.Errorf("result[%d] length = %v, want 3", i, len(result))
			}
			for j, val := range result {
				if val != input.Values[j] {
					t.Errorf("result[%d][%d] = %v, want %v", i, j, val, input.Values[j])
				}
			}
		}
	})
}
