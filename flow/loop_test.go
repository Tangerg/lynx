package flow

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// TestLoopConfig_validate tests the validation logic for LoopConfig
func TestLoopConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *LoopConfig[int]
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "loop config cannot be nil",
		},
		{
			name: "nil processor",
			config: &LoopConfig[int]{
				Processor:     nil,
				MaxIterations: 10,
			},
			wantErr: true,
			errMsg:  "loop processor cannot be nil",
		},
		{
			name: "valid config",
			config: &LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val, false, nil
				},
				MaxIterations: 10,
			},
			wantErr: false,
		},
		{
			name: "valid config with zero max iterations",
			config: &LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val, false, nil
				},
				MaxIterations: math.MaxInt16,
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

// TestNewLoop tests the constructor for Loop
func TestNewLoop(t *testing.T) {
	tests := []struct {
		name    string
		config  LoopConfig[int]
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val * 2, i >= 5, nil
				},
				MaxIterations: 10,
			},
			wantErr: false,
		},
		{
			name: "nil processor",
			config: LoopConfig[int]{
				Processor:     nil,
				MaxIterations: 10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := NewLoop(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewLoop() expected error but got nil")
				}
				if loop != nil {
					t.Errorf("NewLoop() expected nil loop on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewLoop() unexpected error = %v", err)
				}
				if loop == nil {
					t.Errorf("NewLoop() returned nil loop")
				}
			}
		})
	}
}

// TestLoop_Run tests the main execution logic
func TestLoop_Run(t *testing.T) {
	tests := []struct {
		name      string
		config    LoopConfig[int]
		input     int
		want      int
		wantErr   bool
		errSubstr string
	}{
		{
			name: "simple doubling until > 100",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					newVal := val * 2
					return newVal, newVal > 100, nil
				},
				MaxIterations: 10,
			},
			input:   1,
			want:    128, // 1 → 2 → 4 → 8 → 16 → 32 → 64 → 128
			wantErr: false,
		},
		{
			name: "immediate completion",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val + 1, true, nil
				},
				MaxIterations: 10,
			},
			input:   5,
			want:    6,
			wantErr: false,
		},
		{
			name: "error in processor",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					if i == 3 {
						return val, false, errors.New("processing error")
					}
					return val + 1, false, nil
				},
				MaxIterations: 10,
			},
			input:     0,
			want:      3,
			wantErr:   true,
			errSubstr: "loop failed at iteration 3",
		},
		{
			name: "max iterations exceeded",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val + 1, false, nil // Never terminates naturally
				},
				MaxIterations: 5,
			},
			input:     0,
			want:      5,
			wantErr:   true,
			errSubstr: "loop exceeded max iterations (5)",
		},
		{
			name: "no max iterations limit",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					return val + 1, i >= 100, nil
				},
				MaxIterations: math.MaxInt16,
			},
			input:   0,
			want:    101,
			wantErr: false,
		},
		{
			name: "iteration counter works correctly",
			config: LoopConfig[int]{
				Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
					// Store iteration number in value
					return i, i >= 10, nil
				},
				MaxIterations: 20,
			},
			input:   0,
			want:    10,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := NewLoop(tt.config)
			if err != nil {
				t.Fatalf("NewLoop() error = %v", err)
			}

			ctx := context.Background()
			got, err := loop.Run(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error but got nil")
				} else if tt.errSubstr != "" {
					if !contains(err.Error(), tt.errSubstr) {
						t.Errorf("Run() error = %v, want substring %v", err.Error(), tt.errSubstr)
					}
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

// TestLoop_RunWithContext tests context cancellation
func TestLoop_RunWithContext(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		config := LoopConfig[int]{
			Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
				// Check context cancellation
				select {
				case <-ctx.Done():
					return val, false, ctx.Err()
				default:
				}

				// Cancel after 3 iterations
				if i == 3 {
					cancel()
				}

				return val + 1, false, nil
			},
			MaxIterations: 100,
		}

		loop, err := NewLoop(config)
		if err != nil {
			t.Fatalf("NewLoop() error = %v", err)
		}

		_, err = loop.Run(ctx, 0)
		if err == nil {
			t.Errorf("Run() expected context cancellation error")
		}
		if !errors.Is(err, context.Canceled) && !contains(err.Error(), "context canceled") {
			t.Errorf("Run() error should be context.Canceled, got %v", err)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		config := LoopConfig[int]{
			Processor: func(ctx context.Context, i int, val int) (int, bool, error) {
				// Check context
				select {
				case <-ctx.Done():
					return val, false, ctx.Err()
				default:
				}

				// Simulate slow processing
				time.Sleep(20 * time.Millisecond)
				return val + 1, false, nil
			},
			MaxIterations: 100,
		}

		loop, err := NewLoop(config)
		if err != nil {
			t.Fatalf("NewLoop() error = %v", err)
		}

		_, err = loop.Run(ctx, 0)
		if err == nil {
			t.Errorf("Run() expected timeout error")
		}
	})
}

// TestLoopBuilder tests the builder pattern
func TestLoopBuilder(t *testing.T) {
	t.Run("complete builder chain", func(t *testing.T) {
		loop, err := NewLoopBuilder[int]().
			WithMaxIterations(10).
			WithProcessor(func(ctx context.Context, i int, input int) (int, bool, error) {
				return input * 2, input*2 > 50, nil
			}).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		result, err := loop.Run(ctx, 1)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result != 64 {
			t.Errorf("Run() got = %v, want 64", result)
		}
	})

	t.Run("builder without processor", func(t *testing.T) {
		_, err := NewLoopBuilder[any]().
			WithMaxIterations(10).
			Build()

		if err == nil {
			t.Errorf("Build() expected error for missing processor")
		}
	})

	t.Run("builder with type assertion", func(t *testing.T) {
		loop, err := NewLoopBuilder[string]().
			WithProcessor(func(ctx context.Context, i int, str string) (string, bool, error) {
				return str + "x", len(str) >= 5, nil
			}).
			WithMaxIterations(math.MaxInt16).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		result, err := loop.Run(ctx, "a")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result != "axxxxx" {
			t.Errorf("Run() got = %v, want 'axxxxx'", result)
		}
	})
}

// TestLoop_ComplexScenarios tests more complex use cases
func TestLoop_ComplexScenarios(t *testing.T) {
	t.Run("fibonacci until > 100", func(t *testing.T) {
		type FibState struct {
			prev, curr int
		}

		config := LoopConfig[FibState]{
			Processor: func(ctx context.Context, i int, state FibState) (FibState, bool, error) {
				next := state.prev + state.curr
				return FibState{prev: state.curr, curr: next}, next > 100, nil
			},
			MaxIterations: 20,
		}

		loop, err := NewLoop(config)
		if err != nil {
			t.Fatalf("NewLoop() error = %v", err)
		}

		result, err := loop.Run(context.Background(), FibState{prev: 0, curr: 1})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		// First fibonacci > 100 is 144
		if result.curr != 144 {
			t.Errorf("Run() got = %v, want 144", result.curr)
		}
	})

	t.Run("retry with exponential backoff", func(t *testing.T) {
		type RetryState struct {
			attempt int
			success bool
		}

		attempts := 0
		config := LoopConfig[RetryState]{
			Processor: func(ctx context.Context, i int, state RetryState) (RetryState, bool, error) {
				attempts++
				// Succeed on 3rd attempt
				if attempts >= 3 {
					return RetryState{attempt: attempts, success: true}, true, nil
				}
				return RetryState{attempt: attempts, success: false}, false, nil
			},
			MaxIterations: 5,
		}

		loop, err := NewLoop(config)
		if err != nil {
			t.Fatalf("NewLoop() error = %v", err)
		}

		result, err := loop.Run(context.Background(), RetryState{})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result.attempt != 3 {
			t.Errorf("Run() attempts = %v, want 3", result.attempt)
		}
		if !result.success {
			t.Errorf("Run() success = false, want true")
		}
	})

	t.Run("state machine", func(t *testing.T) {
		type State string
		const (
			StateInit       State = "init"
			StateProcessing State = "processing"
			StateValidating State = "validating"
			StateCompleted  State = "completed"
		)

		config := LoopConfig[State]{
			Processor: func(ctx context.Context, i int, current State) (State, bool, error) {
				switch current {
				case StateInit:
					return StateProcessing, false, nil
				case StateProcessing:
					return StateValidating, false, nil
				case StateValidating:
					return StateCompleted, false, nil
				case StateCompleted:
					return StateCompleted, true, nil
				default:
					return current, false, errors.New("unknown state")
				}
			},
			MaxIterations: 10,
		}

		loop, err := NewLoop(config)
		if err != nil {
			t.Fatalf("NewLoop() error = %v", err)
		}

		result, err := loop.Run(context.Background(), StateInit)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		if result != StateCompleted {
			t.Errorf("Run() final state = %v, want %v", result, StateCompleted)
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
