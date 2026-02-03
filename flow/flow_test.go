package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ========================================
// Flow Constructor Tests
// ========================================

func TestNewFlow(t *testing.T) {
	flow := NewFlow()
	if flow == nil {
		t.Fatal("NewFlow() returned nil")
	}
	if len(flow.nodes) != 0 {
		t.Errorf("NewFlow() nodes = %d, want 0", len(flow.nodes))
	}
	if len(flow.errors) != 0 {
		t.Errorf("NewFlow() errors = %d, want 0", len(flow.errors))
	}
}

// ========================================
// Flow.Then Tests
// ========================================

func TestFlow_Then(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []Node[any, any]
		wantCount int
	}{
		{
			name: "single node",
			nodes: []Node[any, any]{
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				}),
			},
			wantCount: 1,
		},
		{
			name: "multiple nodes",
			nodes: []Node[any, any]{
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				}),
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				}),
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) - 5, nil
				}),
			},
			wantCount: 3,
		},
		{
			name: "nil node ignored",
			nodes: []Node[any, any]{
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				}),
				nil,
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				}),
			},
			wantCount: 2,
		},
		{
			name: "multiple nil nodes",
			nodes: []Node[any, any]{
				nil,
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				}),
				nil,
				nil,
				Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				}),
				nil,
			},
			wantCount: 2,
		},
		{
			name: "all nil nodes",
			nodes: []Node[any, any]{
				nil,
				nil,
				nil,
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := NewFlow()
			for _, node := range tt.nodes {
				flow.Then(node)
			}

			if len(flow.nodes) != tt.wantCount {
				t.Errorf("Then() nodes count = %d, want %d", len(flow.nodes), tt.wantCount)
			}
			if len(flow.errors) != 0 {
				t.Errorf("Then() should not accumulate errors, got %d", len(flow.errors))
			}
		})
	}
}

func TestFlow_Then_Chaining(t *testing.T) {
	flow := NewFlow()
	result := flow.Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	}))

	if result != flow {
		t.Error("Then() should return the same flow instance for chaining")
	}
}

// ========================================
// Flow.Loop Tests
// ========================================

func TestFlow_Loop(t *testing.T) {
	t.Run("valid loop configuration", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) + 1
					return val, val >= 10, nil
				}).
					WithMaxIterations(100)
			})

		if len(flow.nodes) != 1 {
			t.Errorf("Loop() nodes count = %d, want 1", len(flow.nodes))
		}
		if len(flow.errors) != 0 {
			t.Errorf("Loop() accumulated %d errors, want 0", len(flow.errors))
		}
	})

	t.Run("invalid loop - missing processor", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithMaxIterations(10)
			})

		if len(flow.errors) == 0 {
			t.Error("Loop() should accumulate error for missing processor")
		}
		if len(flow.nodes) != 0 {
			t.Errorf("Loop() should not add node on error, got %d nodes", len(flow.nodes))
		}
	})

	t.Run("invalid loop - missing max iterations", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					return input, false, nil
				})
			})

		if len(flow.errors) > 0 {
			t.Error("Loop() should accumulate error for missing max iterations")
		}
	})

	t.Run("loop chaining", func(t *testing.T) {
		flow := NewFlow()
		result := flow.Loop(func(b *LoopBuilder[any]) {
			b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
				return input, true, nil
			}).WithMaxIterations(10)
		})

		if result != flow {
			t.Error("Loop() should return the same flow instance for chaining")
		}
	})
}

// ========================================
// Flow.Branch Tests
// ========================================

func TestFlow_Branch(t *testing.T) {
	t.Run("valid branch configuration", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"high": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					"low": func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int) > 5 {
							return "high"
						}
						return "low"
					})
			})

		if len(flow.nodes) != 1 {
			t.Errorf("Branch() nodes count = %d, want 1", len(flow.nodes))
		}
		if len(flow.errors) != 0 {
			t.Errorf("Branch() accumulated %d errors, want 0", len(flow.errors))
		}
	})

	t.Run("invalid branch - missing branches", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranchResolver(func(ctx context.Context, input any) string {
					return "test"
				})
			})

		if len(flow.errors) == 0 {
			t.Error("Branch() should accumulate error for missing branches")
		}
		if len(flow.nodes) != 0 {
			t.Errorf("Branch() should not add node on error, got %d nodes", len(flow.nodes))
		}
	})

	t.Run("invalid branch - missing resolver", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"test": func(ctx context.Context, input any) (any, error) {
						return input, nil
					},
					"test1": func(ctx context.Context, input any) (any, error) {
						return input, nil
					},
				})
			})

		if len(flow.errors) == 0 {
			t.Error("Branch() should accumulate error for missing resolver")
		}
	})

	t.Run("invalid branch - empty branches map", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						return "test"
					})
			})

		if len(flow.errors) == 0 {
			t.Error("Branch() should accumulate error for empty branches")
		}
	})

	t.Run("branch chaining", func(t *testing.T) {
		flow := NewFlow()
		result := flow.Branch(func(b *BranchBuilder[any, any]) {
			b.WithBranches(map[string]func(context.Context, any) (any, error){
				"test": func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			}).WithBranchResolver(func(ctx context.Context, input any) string {
				return "test"
			})
		})

		if result != flow {
			t.Error("Branch() should return the same flow instance for chaining")
		}
	})
}

// ========================================
// Flow.Iteration Tests
// ========================================

func TestFlow_Iteration(t *testing.T) {
	t.Run("valid iteration configuration", func(t *testing.T) {
		flow := NewFlow().
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input.(int) * 2, nil
				}).
					WithConcurrencyLimit(3)
			})

		if len(flow.nodes) != 1 {
			t.Errorf("Iteration() nodes count = %d, want 1", len(flow.nodes))
		}
		if len(flow.errors) != 0 {
			t.Errorf("Iteration() accumulated %d errors, want 0", len(flow.errors))
		}
	})

	t.Run("invalid iteration - missing processor", func(t *testing.T) {
		flow := NewFlow().
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithConcurrencyLimit(3)
			})

		if len(flow.errors) == 0 {
			t.Error("Iteration() should accumulate error for missing processor")
		}
		if len(flow.nodes) != 0 {
			t.Errorf("Iteration() should not add node on error, got %d nodes", len(flow.nodes))
		}
	})

	t.Run("iteration with wrong input type", func(t *testing.T) {
		flow := NewFlow().
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input.(int) * 2, nil
				})
			})

		pipeline, err := flow.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		// Pass non-slice input
		_, err = pipeline.Run(context.Background(), 42)
		if err == nil {
			t.Error("Run() should return error for non-slice input")
		}
		if !strings.Contains(err.Error(), "iteration expects []any input") {
			t.Errorf("Run() error = %v, want error about []any input", err)
		}
	})

	t.Run("iteration with string input", func(t *testing.T) {
		flow := NewFlow().
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input, nil
				})
			})

		pipeline, _ := flow.Build()

		_, err := pipeline.Run(context.Background(), "not a slice")
		if err == nil {
			t.Error("Run() should return error for string input")
		}
		if !strings.Contains(err.Error(), "iteration expects []any input, got string") {
			t.Errorf("Run() error = %v, want type mismatch error", err)
		}
	})

	t.Run("iteration chaining", func(t *testing.T) {
		flow := NewFlow()
		result := flow.Iteration(func(b *IterationBuilder[any, any]) {
			b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
				return input, nil
			})
		})

		if result != flow {
			t.Error("Iteration() should return the same flow instance for chaining")
		}
	})
}

// ========================================
// Flow.Parallel Tests
// ========================================

func TestFlow_Parallel(t *testing.T) {
	t.Run("valid parallel configuration", func(t *testing.T) {
		flow := NewFlow().
			Parallel(func(b *ParallelBuilder[any, any]) {
				b.WithProcessors([]func(context.Context, any) (any, error){
					func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
				})
			})

		if len(flow.nodes) != 1 {
			t.Errorf("Parallel() nodes count = %d, want 1", len(flow.nodes))
		}
		if len(flow.errors) != 0 {
			t.Errorf("Parallel() accumulated %d errors, want 0", len(flow.errors))
		}
	})

	t.Run("invalid parallel - missing processors", func(t *testing.T) {
		flow := NewFlow().
			Parallel(func(b *ParallelBuilder[any, any]) {
				b.WithConcurrencyLimit(3)
			})

		if len(flow.errors) == 0 {
			t.Error("Parallel() should accumulate error for missing processors")
		}
		if len(flow.nodes) != 0 {
			t.Errorf("Parallel() should not add node on error, got %d nodes", len(flow.nodes))
		}
	})

	t.Run("invalid parallel - empty processors", func(t *testing.T) {
		flow := NewFlow().
			Parallel(func(b *ParallelBuilder[any, any]) {
				b.WithProcessors([]func(context.Context, any) (any, error){})
			})

		if len(flow.errors) == 0 {
			t.Error("Parallel() should accumulate error for empty processors")
		}
	})

	t.Run("parallel chaining", func(t *testing.T) {
		flow := NewFlow()
		result := flow.Parallel(func(b *ParallelBuilder[any, any]) {
			b.WithProcessors([]func(context.Context, any) (any, error){
				func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			})
		})

		if result != flow {
			t.Error("Parallel() should return the same flow instance for chaining")
		}
	})
}

// ========================================
// Flow.validate Tests
// ========================================

func TestFlow_validate(t *testing.T) {
	tests := []struct {
		name    string
		flow    *Flow
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid flow",
			flow: NewFlow().Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input, nil
			})),
			wantErr: false,
		},
		{
			name:    "empty flow",
			flow:    NewFlow(),
			wantErr: true,
			errMsg:  "flow must contain at least one node",
		},
		{
			name: "flow with accumulated errors",
			flow: func() *Flow {
				f := NewFlow()
				f.errors = append(f.errors, errors.New("config error 1"))
				f.nodes = append(f.nodes, Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input, nil
				}))
				return f
			}(),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with multiple errors",
			flow: func() *Flow {
				f := NewFlow()
				f.errors = append(f.errors, errors.New("error 1"))
				f.errors = append(f.errors, errors.New("error 2"))
				f.errors = append(f.errors, errors.New("error 3"))
				f.nodes = append(f.nodes, Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input, nil
				}))
				return f
			}(),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with errors and no nodes",
			flow: func() *Flow {
				f := NewFlow()
				f.errors = append(f.errors, errors.New("config error"))
				return f
			}(),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flow.validate()
			if tt.wantErr {
				if err == nil {
					t.Error("validate() expected error but got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %v, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// ========================================
// Flow.Build Tests
// ========================================

func TestFlow_Build(t *testing.T) {
	tests := []struct {
		name    string
		flow    *Flow
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid single node",
			flow: NewFlow().Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			})),
			wantErr: false,
		},
		{
			name: "valid multiple nodes",
			flow: NewFlow().
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				})),
			wantErr: false,
		},
		{
			name:    "empty flow",
			flow:    NewFlow(),
			wantErr: true,
			errMsg:  "flow must contain at least one node",
		},
		{
			name: "flow with loop error",
			flow: NewFlow().
				Loop(func(b *LoopBuilder[any]) {
					b.WithMaxIterations(10) // Missing processor
				}),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with branch error",
			flow: NewFlow().
				Branch(func(b *BranchBuilder[any, any]) {
					b.WithBranches(map[string]func(context.Context, any) (any, error){
						"test": func(ctx context.Context, input any) (any, error) {
							return input, nil
						},
						"test1": func(ctx context.Context, input any) (any, error) {
							return input, nil
						},
					})
					// Missing resolver
				}),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with iteration error",
			flow: NewFlow().
				Iteration(func(b *IterationBuilder[any, any]) {
					b.WithConcurrencyLimit(3) // Missing processor
				}),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with parallel error",
			flow: NewFlow().
				Parallel(func(b *ParallelBuilder[any, any]) {
					b.WithConcurrencyLimit(3) // Missing processors
				}),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
		{
			name: "flow with multiple errors",
			flow: NewFlow().
				Loop(func(b *LoopBuilder[any]) {
					b.WithMaxIterations(10) // Missing processor
				}).
				Branch(func(b *BranchBuilder[any, any]) {
					// Missing everything
				}),
			wantErr: true,
			errMsg:  "flow configuration failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := tt.flow.Build()
			if tt.wantErr {
				if err == nil {
					t.Error("Build() expected error but got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Build() error = %v, want substring %q", err.Error(), tt.errMsg)
				}
				if pipeline != nil {
					t.Error("Build() expected nil pipeline on error")
				}
			} else {
				if err != nil {
					t.Errorf("Build() unexpected error = %v", err)
				}
				if pipeline == nil {
					t.Error("Build() returned nil pipeline")
				}
			}
		})
	}
}

// ========================================
// Flow Execution Tests
// ========================================

func TestFlow_ExecuteSimple(t *testing.T) {
	tests := []struct {
		name    string
		flow    *Flow
		input   any
		want    any
		wantErr bool
	}{
		{
			name: "single transformation",
			flow: NewFlow().Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			})),
			input: 5,
			want:  10,
		},
		{
			name: "chained transformations",
			flow: NewFlow().
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				})),
			input: 5,
			want:  20, // (5 * 2) + 10
		},
		{
			name: "three chained transformations",
			flow: NewFlow().
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 1, nil
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 3, nil
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) - 5, nil
				})),
			input: 4,
			want:  10, // ((4 + 1) * 3) - 5 = 10
		},
		{
			name: "with error in middle",
			flow: NewFlow().
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return nil, errors.New("processing failed")
				})).
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return input.(int) + 10, nil
				})),
			input:   5,
			wantErr: true,
		},
		{
			name: "with error at start",
			flow: NewFlow().
				Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
					return nil, errors.New("immediate failure")
				})),
			input:   5,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := tt.flow.Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			result, err := pipeline.Run(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("Run() expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
				if result != tt.want {
					t.Errorf("Run() = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func TestFlow_ExecuteWithLoop(t *testing.T) {
	t.Run("simple loop", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) + 1
					return val, val >= 10, nil
				}).
					WithMaxIterations(100)
			})

		pipeline, err := flow.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		result, err := pipeline.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != 10 {
			t.Errorf("Run(5) = %v, want 10", result)
		}
	})

	t.Run("loop with transformation before", func(t *testing.T) {
		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) * 2, nil
			})).
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) + 1
					return val, val >= 20, nil
				}).
					WithMaxIterations(100)
			})

		pipeline, _ := flow.Build()
		result, _ := pipeline.Run(context.Background(), 5)

		// 5 * 2 = 10, then loop to 20
		if result != 20 {
			t.Errorf("Run(5) = %v, want 20", result)
		}
	})

	t.Run("loop with transformation after", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) + 1
					return val, val >= 10, nil
				}).
					WithMaxIterations(100)
			}).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) * 3, nil
			}))

		pipeline, _ := flow.Build()
		result, _ := pipeline.Run(context.Background(), 5)

		// Loop 5 to 10, then 10 * 3 = 30
		if result != 30 {
			t.Errorf("Run(5) = %v, want 30", result)
		}
	})
}

func TestFlow_ExecuteWithBranch(t *testing.T) {
	t.Run("branch with different paths", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"high": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					"low": func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int) > 5 {
							return "high"
						}
						return "low"
					})
			})

		pipeline, err := flow.Build()
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		// Test high branch
		result, _ := pipeline.Run(context.Background(), 10)
		if result != 20 {
			t.Errorf("Run(10) = %v, want 20", result)
		}

		// Test low branch
		result, _ = pipeline.Run(context.Background(), 3)
		if result != 13 {
			t.Errorf("Run(3) = %v, want 13", result)
		}
	})

	t.Run("branch with transformation before", func(t *testing.T) {
		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) + 5, nil
			})).
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"double": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					"triple": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 3, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int) > 10 {
							return "triple"
						}
						return "double"
					})
			})

		pipeline, _ := flow.Build()

		// 5 + 5 = 10, use double: 10 * 2 = 20
		result, _ := pipeline.Run(context.Background(), 5)
		if result != 20 {
			t.Errorf("Run(5) = %v, want 20", result)
		}

		// 8 + 5 = 13, use triple: 13 * 3 = 39
		result, _ = pipeline.Run(context.Background(), 8)
		if result != 39 {
			t.Errorf("Run(8) = %v, want 39", result)
		}
	})
}

func TestFlow_ExecuteWithIteration(t *testing.T) {
	t.Run("simple iteration", func(t *testing.T) {
		flow := NewFlow().
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input.(int) * 2, nil
				})
			})

		pipeline, _ := flow.Build()

		input := []any{1, 2, 3, 4, 5}
		result, err := pipeline.Run(context.Background(), input)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		results, ok := result.([]Result[any])
		if !ok {
			t.Fatalf("result type = %T, want []Result[any]", result)
		}

		expected := []int{2, 4, 6, 8, 10}
		for i, r := range results {
			if r.Error != nil {
				t.Errorf("result[%d] has error: %v", i, r.Error)
			}
			if r.Value.(int) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, r.Value, expected[i])
			}
		}
	})

	t.Run("iteration with transformation before", func(t *testing.T) {
		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				// Convert int to slice
				val := input.(int)
				return []any{val, val + 1, val + 2}, nil
			})).
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input.(int) * 10, nil
				})
			})

		pipeline, _ := flow.Build()

		result, _ := pipeline.Run(context.Background(), 5)
		results := result.([]Result[any])

		expected := []int{50, 60, 70}
		for i, r := range results {
			if r.Value.(int) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, r.Value, expected[i])
			}
		}
	})
}

func TestFlow_ExecuteWithParallel(t *testing.T) {
	t.Run("simple parallel", func(t *testing.T) {
		flow := NewFlow().
			Parallel(func(b *ParallelBuilder[any, any]) {
				b.WithProcessors([]func(context.Context, any) (any, error){
					func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
					func(ctx context.Context, input any) (any, error) {
						return input.(int) * input.(int), nil
					},
				})
			})

		pipeline, _ := flow.Build()

		result, err := pipeline.Run(context.Background(), 5)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}

		results, ok := result.([]Result[any])
		if !ok {
			t.Fatalf("result type = %T, want []Result[any]", result)
		}

		expected := []int{10, 15, 25} // 5*2, 5+10, 5*5
		for i, r := range results {
			if r.Error != nil {
				t.Errorf("result[%d] has error: %v", i, r.Error)
			}
			if r.Value.(int) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, r.Value, expected[i])
			}
		}
	})

	t.Run("parallel with aggregation", func(t *testing.T) {
		flow := NewFlow().
			Parallel(func(b *ParallelBuilder[any, any]) {
				b.WithProcessors([]func(context.Context, any) (any, error){
					func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
					func(ctx context.Context, input any) (any, error) {
						return input.(int) * input.(int), nil
					},
				})
			}).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				results := input.([]Result[any])
				sum := 0
				for _, r := range results {
					if r.Error == nil {
						sum += r.Value.(int)
					}
				}
				return sum, nil
			}))

		pipeline, _ := flow.Build()

		// Input 5: [10, 15, 25] -> sum = 50
		result, _ := pipeline.Run(context.Background(), 5)
		if result != 50 {
			t.Errorf("Run(5) = %v, want 50", result)
		}
	})
}

// ========================================
// Flow Complex Scenarios
// ========================================

func TestFlow_ComplexScenarios(t *testing.T) {
	t.Run("mixed control flow", func(t *testing.T) {
		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) + 1, nil
			})).
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) * 2
					return val, val >= 10, nil
				}).
					WithMaxIterations(10)
			}).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) - 5, nil
			}))

		pipeline, _ := flow.Build()

		// 2 -> 3 (add 1) -> 6 (double) -> 12 (double, >= 10) -> 7 (sub 5)
		result, _ := pipeline.Run(context.Background(), 2)
		if result != 7 {
			t.Errorf("Run(2) = %v, want 7", result)
		}
	})

	t.Run("branch with loop", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"increment": func(ctx context.Context, input any) (any, error) {
						return input.(int) + 1, nil
					},
					"double": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int)%2 == 0 {
							return "double"
						}
						return "increment"
					})
			}).
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					val := input.(int) + 1
					return val, val >= 20, nil
				}).
					WithMaxIterations(100)
			})

		pipeline, _ := flow.Build()

		// Even: 4 -> 8 (double) -> loop to 20
		result, _ := pipeline.Run(context.Background(), 4)
		if result != 20 {
			t.Errorf("Run(4) = %v, want 20", result)
		}

		// Odd: 5 -> 6 (increment) -> loop to 20
		result, _ = pipeline.Run(context.Background(), 5)
		if result != 20 {
			t.Errorf("Run(5) = %v, want 20", result)
		}
	})

	t.Run("all node types together", func(t *testing.T) {
		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				// Initial transformation
				return input.(int) + 2, nil
			})).
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"to_slice": func(ctx context.Context, input any) (any, error) {
						val := input.(int)
						return []any{val, val + 1, val + 2}, nil
					},
					"single": func(ctx context.Context, input any) (any, error) {
						return []any{input.(int)}, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int) > 10 {
							return "to_slice"
						}
						return "single"
					})
			}).
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithProcessor(func(ctx context.Context, i int, input any) (any, error) {
					return input.(int) * 2, nil
				})
			})

		pipeline, _ := flow.Build()

		// Input 3: 3+2=5 (< 10) -> [5] -> [10]
		result, _ := pipeline.Run(context.Background(), 3)
		results := result.([]Result[any])
		if len(results) != 1 || results[0].Value.(int) != 10 {
			t.Errorf("Run(3) got unexpected result")
		}

		// Input 9: 9+2=11 (> 10) -> [11,12,13] -> [22,24,26]
		result, _ = pipeline.Run(context.Background(), 9)
		results = result.([]Result[any])
		if len(results) != 3 {
			t.Errorf("Run(9) got %d results, want 3", len(results))
		}
		expected := []int{22, 24, 26}
		for i, r := range results {
			if r.Value.(int) != expected[i] {
				t.Errorf("result[%d] = %v, want %v", i, r.Value, expected[i])
			}
		}
	})
}

// ========================================
// Error Handling Tests
// ========================================

func TestFlow_ErrorPropagation(t *testing.T) {
	t.Run("error stops execution", func(t *testing.T) {
		var callCount int32

		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				atomic.AddInt32(&callCount, 1)
				return input.(int) * 2, nil
			})).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				atomic.AddInt32(&callCount, 1)
				return nil, fmt.Errorf("error at node 2")
			})).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				atomic.AddInt32(&callCount, 1)
				return input.(int) + 5, nil
			}))

		pipeline, _ := flow.Build()
		_, err := pipeline.Run(context.Background(), 5)

		if err == nil {
			t.Error("Run() expected error")
		}

		// Only first two nodes should be called
		if atomic.LoadInt32(&callCount) != 2 {
			t.Errorf("callCount = %d, want 2", callCount)
		}
	})

	t.Run("multiple configuration errors", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithMaxIterations(10) // Missing processor
			}).
			Branch(func(b *BranchBuilder[any, any]) {
				// Missing everything
			}).
			Iteration(func(b *IterationBuilder[any, any]) {
				b.WithConcurrencyLimit(3) // Missing processor
			})

		_, err := flow.Build()
		if err == nil {
			t.Error("Build() expected error for multiple configuration failures")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "flow configuration failed") {
			t.Errorf("Build() error should mention configuration failure")
		}
	})

	t.Run("error in branch", func(t *testing.T) {
		flow := NewFlow().
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"success": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					"fail": func(ctx context.Context, input any) (any, error) {
						return nil, errors.New("branch failed")
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int) > 5 {
							return "fail"
						}
						return "success"
					})
			})

		pipeline, _ := flow.Build()

		// Success path
		result, err := pipeline.Run(context.Background(), 3)
		if err != nil {
			t.Errorf("Run(3) unexpected error: %v", err)
		}
		if result != 6 {
			t.Errorf("Run(3) = %v, want 6", result)
		}

		// Fail path
		_, err = pipeline.Run(context.Background(), 10)
		if err == nil {
			t.Error("Run(10) expected error")
		}
	})

	t.Run("error in loop iteration", func(t *testing.T) {
		flow := NewFlow().
			Loop(func(b *LoopBuilder[any]) {
				b.WithProcessor(func(ctx context.Context, iteration int, input any) (any, bool, error) {
					if iteration >= 3 {
						return nil, false, errors.New("loop failed at iteration 3")
					}
					return input.(int) + 1, false, nil
				}).
					WithMaxIterations(10)
			})

		pipeline, _ := flow.Build()
		_, err := pipeline.Run(context.Background(), 5)

		if err == nil {
			t.Error("Run() expected error from loop")
		}
	})
}

// ========================================
// Context Handling Tests
// ========================================

func TestFlow_ContextHandling(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				select {
				case <-time.After(100 * time.Millisecond):
					return input.(int) * 2, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}))

		pipeline, _ := flow.Build()
		_, err := pipeline.Run(ctx, 5)

		if err == nil {
			t.Error("Run() expected context error")
		}
	})

	t.Run("context values preserved", func(t *testing.T) {
		type contextKey string
		key := contextKey("test_key")

		flow := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				val := ctx.Value(key)
				if val == nil {
					return nil, errors.New("context value lost")
				}
				return input.(int) + val.(int), nil
			}))

		pipeline, _ := flow.Build()

		ctx := context.WithValue(context.Background(), key, 10)
		result, err := pipeline.Run(ctx, 5)

		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != 15 {
			t.Errorf("Run() = %v, want 15", result)
		}
	})
}

// ========================================
// Flow Method Chaining Tests
// ========================================

func TestFlow_MethodChaining(t *testing.T) {
	t.Run("complete fluent chain", func(t *testing.T) {
		pipeline, err := NewFlow().
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) + 1, nil
			})).
			Branch(func(b *BranchBuilder[any, any]) {
				b.WithBranches(map[string]func(context.Context, any) (any, error){
					"even": func(ctx context.Context, input any) (any, error) {
						return input.(int) * 2, nil
					},
					"odd": func(ctx context.Context, input any) (any, error) {
						return input.(int) + 10, nil
					},
				}).
					WithBranchResolver(func(ctx context.Context, input any) string {
						if input.(int)%2 == 0 {
							return "even"
						}
						return "odd"
					})
			}).
			Then(Func[any, any](func(ctx context.Context, input any) (any, error) {
				return input.(int) - 5, nil
			})).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		// Test even path: 5 -> 6 (add 1) -> 12 (even * 2) -> 7 (sub 5)
		result, _ := pipeline.Run(context.Background(), 5)
		if result != 7 {
			t.Errorf("Run(5) = %v, want 7", result)
		}

		// Test odd path: 4 -> 5 (add 1) -> 15 (odd + 10) -> 10 (sub 5)
		result, _ = pipeline.Run(context.Background(), 4)
		if result != 10 {
			t.Errorf("Run(4) = %v, want 10", result)
		}
	})
}
