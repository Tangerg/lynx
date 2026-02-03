package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestBranchConfig_validate tests the validation logic for BranchConfig.
// Type: BranchConfig[int, string] - validates configuration rules for branches
func TestBranchConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *BranchConfig[int, string]
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "branch config cannot be nil",
		},
		{
			name: "empty branches",
			config: &BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){},
				BranchResolver: func(ctx context.Context, i int) string {
					return "default"
				},
			},
			wantErr: true,
			errMsg:  "at least one branch is required",
		},
		{
			name: "nil resolver with multiple branches",
			config: &BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"branch_a": func(ctx context.Context, i int) (string, error) {
						return "A", nil
					},
					"branch_b": func(ctx context.Context, i int) (string, error) {
						return "B", nil
					},
				},
				BranchResolver: nil,
			},
			wantErr: true,
			errMsg:  "branch resolver cannot be nil",
		},
		{
			name: "valid config with resolver",
			config: &BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"positive": func(ctx context.Context, i int) (string, error) {
						return "positive", nil
					},
					"negative": func(ctx context.Context, i int) (string, error) {
						return "negative", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					if i >= 0 {
						return "positive"
					}
					return "negative"
				},
			},
			wantErr: false,
		},
		{
			name: "single branch without resolver (should auto-set)",
			config: &BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"only_branch": func(ctx context.Context, i int) (string, error) {
						return "result", nil
					},
				},
				BranchResolver: nil,
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
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
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

// TestNewBranch tests the constructor for Branch.
// Type: Branch[int, string] - verifies proper initialization and error handling
func TestNewBranch(t *testing.T) {
	tests := []struct {
		name    string
		config  BranchConfig[int, string]
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"even": func(ctx context.Context, i int) (string, error) {
						return "even", nil
					},
					"odd": func(ctx context.Context, i int) (string, error) {
						return "odd", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					if i%2 == 0 {
						return "even"
					}
					return "odd"
				},
			},
			wantErr: false,
		},
		{
			name: "empty branches",
			config: BranchConfig[int, string]{
				Branches:       map[string]func(context.Context, int) (string, error){},
				BranchResolver: func(ctx context.Context, i int) string { return "x" },
			},
			wantErr: true,
		},
		{
			name: "nil resolver",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"branch": func(ctx context.Context, i int) (string, error) {
						return "result", nil
					},
				},
				BranchResolver: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, err := NewBranch(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewBranch() expected error but got nil")
				}
				if branch != nil {
					t.Errorf("NewBranch() expected nil branch on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewBranch() unexpected error = %v", err)
				}
				if branch == nil {
					t.Errorf("NewBranch() returned nil branch")
				}
				// Verify branches were cloned
				if len(branch.branches) != len(tt.config.Branches) {
					t.Errorf("NewBranch() branches count = %v, want %v",
						len(branch.branches), len(tt.config.Branches))
				}
			}
		})
	}
}

// TestBranch_Run tests the main execution logic.
// Type: Branch[int, string] - validates branch resolution and execution
func TestBranch_Run(t *testing.T) {
	tests := []struct {
		name      string
		config    BranchConfig[int, string]
		input     int
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "positive number routing",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"positive": func(ctx context.Context, i int) (string, error) {
						return fmt.Sprintf("positive: %d", i), nil
					},
					"negative": func(ctx context.Context, i int) (string, error) {
						return fmt.Sprintf("negative: %d", i), nil
					},
					"zero": func(ctx context.Context, i int) (string, error) {
						return "zero", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					if i > 0 {
						return "positive"
					} else if i < 0 {
						return "negative"
					}
					return "zero"
				},
			},
			input:   42,
			want:    "positive: 42",
			wantErr: false,
		},
		{
			name: "negative number routing",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"positive": func(ctx context.Context, i int) (string, error) {
						return "positive", nil
					},
					"negative": func(ctx context.Context, i int) (string, error) {
						return "negative", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					if i >= 0 {
						return "positive"
					}
					return "negative"
				},
			},
			input:   -10,
			want:    "negative",
			wantErr: false,
		},
		{
			name: "branch not found",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"existing": func(ctx context.Context, i int) (string, error) {
						return "result", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					return "non_existent"
				},
			},
			input:     1,
			wantErr:   true,
			errSubstr: "branch 'non_existent' not found",
		},
		{
			name: "branch execution error",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"error_branch": func(ctx context.Context, i int) (string, error) {
						return "", errors.New("processing failed")
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					return "error_branch"
				},
			},
			input:     1,
			wantErr:   true,
			errSubstr: "branch execution failed",
		},
		{
			name: "even/odd routing",
			config: BranchConfig[int, string]{
				Branches: map[string]func(context.Context, int) (string, error){
					"even": func(ctx context.Context, i int) (string, error) {
						return "even", nil
					},
					"odd": func(ctx context.Context, i int) (string, error) {
						return "odd", nil
					},
				},
				BranchResolver: func(ctx context.Context, i int) string {
					if i%2 == 0 {
						return "even"
					}
					return "odd"
				},
			},
			input:   7,
			want:    "odd",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch, err := NewBranch(tt.config)
			if err != nil {
				t.Fatalf("NewBranch() error = %v", err)
			}

			ctx := context.Background()
			got, err := branch.Run(ctx, tt.input)

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

// TestBranch_resolveBranch tests the branch resolution logic.
// Type: Branch[string, string] - validates resolver function and error handling
func TestBranch_resolveBranch(t *testing.T) {
	t.Run("successful resolution", func(t *testing.T) {
		// Type: Branch[string, string]
		branch, err := NewBranch(BranchConfig[string, string]{
			Branches: map[string]func(context.Context, string) (string, error){
				"route_a": func(ctx context.Context, s string) (string, error) {
					return "A", nil
				},
				"route_b": func(ctx context.Context, s string) (string, error) {
					return "B", nil
				},
			},
			BranchResolver: func(ctx context.Context, s string) string {
				return "route_a"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.Background()
		fn, err := branch.resolveBranch(ctx, "test")
		if err != nil {
			t.Errorf("resolveBranch() unexpected error = %v", err)
		}
		if fn == nil {
			t.Errorf("resolveBranch() returned nil function")
		}
	})

	t.Run("branch not found with available branches in error", func(t *testing.T) {
		// Type: Branch[string, string]
		branch, err := NewBranch(BranchConfig[string, string]{
			Branches: map[string]func(context.Context, string) (string, error){
				"branch_x": func(ctx context.Context, s string) (string, error) {
					return "X", nil
				},
				"branch_y": func(ctx context.Context, s string) (string, error) {
					return "Y", nil
				},
			},
			BranchResolver: func(ctx context.Context, s string) string {
				return "branch_z"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.Background()
		_, err = branch.resolveBranch(ctx, "test")
		if err == nil {
			t.Errorf("resolveBranch() expected error but got nil")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "branch_z") {
			t.Errorf("error message should contain requested branch 'branch_z', got: %v", errMsg)
		}
		if !strings.Contains(errMsg, "available branches") {
			t.Errorf("error message should list available branches, got: %v", errMsg)
		}
	})
}

// TestBranch_RunWithContext tests context handling in branch execution.
// Type: Branch[int, string] - validates context propagation and cancellation
func TestBranch_RunWithContext(t *testing.T) {
	t.Run("context passed to resolver", func(t *testing.T) {
		// Type: Branch[int, string] with context value propagation
		type contextKey string
		key := contextKey("test_key")

		branch, err := NewBranch(BranchConfig[int, string]{
			Branches: map[string]func(context.Context, int) (string, error){
				"branch_a": func(ctx context.Context, i int) (string, error) {
					return "A", nil
				},
			},
			BranchResolver: func(ctx context.Context, i int) string {
				if ctx.Value(key) != nil {
					return "branch_a"
				}
				return "unknown"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.WithValue(context.Background(), key, "test_value")
		result, err := branch.Run(ctx, 1)
		if err != nil {
			t.Errorf("Run() unexpected error = %v", err)
		}
		if result != "A" {
			t.Errorf("Run() got = %v, want A", result)
		}
	})

	t.Run("context passed to branch function", func(t *testing.T) {
		// Type: Branch[int, string] with context data retrieval
		type contextKey string
		key := contextKey("data")

		branch, err := NewBranch(BranchConfig[int, string]{
			Branches: map[string]func(context.Context, int) (string, error){
				"main": func(ctx context.Context, i int) (string, error) {
					val := ctx.Value(key)
					if val != nil {
						return val.(string), nil
					}
					return "no_context", nil
				},
			},
			BranchResolver: func(ctx context.Context, i int) string {
				return "main"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.WithValue(context.Background(), key, "context_data")
		result, err := branch.Run(ctx, 1)
		if err != nil {
			t.Errorf("Run() unexpected error = %v", err)
		}
		if result != "context_data" {
			t.Errorf("Run() got = %v, want context_data", result)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Type: Branch[int, string] with context cancellation handling
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		branch, err := NewBranch(BranchConfig[int, string]{
			Branches: map[string]func(context.Context, int) (string, error){
				"main": func(ctx context.Context, i int) (string, error) {
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					default:
						return "success", nil
					}
				},
			},
			BranchResolver: func(ctx context.Context, i int) string {
				return "main"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		_, err = branch.Run(ctx, 1)
		if err == nil {
			t.Errorf("Run() expected cancellation error")
		}
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Run() error should be context.Canceled, got %v", err)
		}
	})
}

// TestBranchBuilder tests the builder pattern for branch construction.
// Type: BranchBuilder[any, any] - validates fluent API and error accumulation
func TestBranchBuilder(t *testing.T) {
	t.Run("complete builder chain", func(t *testing.T) {
		// Type: BranchBuilder[any, any] -> Branch[any, any]
		branch, err := NewBranchBuilder[any, any]().
			WithBranches(map[string]func(context.Context, any) (any, error){
				"upper": func(ctx context.Context, input any) (any, error) {
					return strings.ToUpper(input.(string)), nil
				},
				"lower": func(ctx context.Context, input any) (any, error) {
					return strings.ToLower(input.(string)), nil
				},
			}).
			WithBranchResolver(func(ctx context.Context, input any) string {
				s := input.(string)
				if s == strings.ToUpper(s) {
					return "lower"
				}
				return "upper"
			}).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()
		result, err := branch.Run(ctx, "hello")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result.(string) != "HELLO" {
			t.Errorf("Run() got = %v, want HELLO", result)
		}
	})

	t.Run("builder without branches", func(t *testing.T) {
		// Type: BranchBuilder[any, any] - missing branches validation
		_, err := NewBranchBuilder[any, any]().
			WithBranchResolver(func(ctx context.Context, input any) string {
				return "x"
			}).
			Build()

		if err == nil {
			t.Errorf("Build() expected error for missing branches")
		}
	})

	t.Run("builder without resolver", func(t *testing.T) {
		// Type: BranchBuilder[any, any] - single branch auto-resolver
		_, err := NewBranchBuilder[any, any]().
			WithBranches(map[string]func(context.Context, any) (any, error){
				"branch": func(ctx context.Context, input any) (any, error) {
					return input, nil
				},
			}).
			Build()

		if err != nil {
			t.Errorf("Build() unexpected error for single branch without resolver: %v", err)
		}
	})

	t.Run("builder with type assertions", func(t *testing.T) {
		// Type: BranchBuilder[any, any] with runtime type checking
		branch, err := NewBranchBuilder[any, any]().
			WithBranches(map[string]func(context.Context, any) (any, error){
				"int": func(ctx context.Context, input any) (any, error) {
					return input.(int) * 2, nil
				},
				"string": func(ctx context.Context, input any) (any, error) {
					return input.(string) + "_suffix", nil
				},
			}).
			WithBranchResolver(func(ctx context.Context, input any) string {
				switch input.(type) {
				case int:
					return "int"
				case string:
					return "string"
				default:
					return "unknown"
				}
			}).
			Build()

		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		ctx := context.Background()

		// Test integer branch: any (int) -> any (int)
		result1, err := branch.Run(ctx, 10)
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result1.(int) != 20 {
			t.Errorf("Run() got = %v, want 20", result1)
		}

		// Test string branch: any (string) -> any (string)
		result2, err := branch.Run(ctx, "prefix")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result2.(string) != "prefix_suffix" {
			t.Errorf("Run() got = %v, want prefix_suffix", result2)
		}
	})
}

// TestBranch_ComplexScenarios tests real-world use cases with domain models.
func TestBranch_ComplexScenarios(t *testing.T) {
	t.Run("user role routing", func(t *testing.T) {
		// Type: Branch[User, Response] - role-based access control
		type User struct {
			ID   int
			Role string
		}

		type Response struct {
			Message string
			Access  string
		}

		branch, err := NewBranch(BranchConfig[User, Response]{
			Branches: map[string]func(context.Context, User) (Response, error){
				"admin": func(ctx context.Context, u User) (Response, error) {
					return Response{
						Message: fmt.Sprintf("Admin user %d", u.ID),
						Access:  "full",
					}, nil
				},
				"user": func(ctx context.Context, u User) (Response, error) {
					return Response{
						Message: fmt.Sprintf("Regular user %d", u.ID),
						Access:  "limited",
					}, nil
				},
				"guest": func(ctx context.Context, u User) (Response, error) {
					return Response{
						Message: "Guest access",
						Access:  "read-only",
					}, nil
				},
			},
			BranchResolver: func(ctx context.Context, u User) string {
				return u.Role
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		tests := []struct {
			user User
			want Response
		}{
			{
				user: User{ID: 1, Role: "admin"},
				want: Response{Message: "Admin user 1", Access: "full"},
			},
			{
				user: User{ID: 2, Role: "user"},
				want: Response{Message: "Regular user 2", Access: "limited"},
			},
			{
				user: User{ID: 3, Role: "guest"},
				want: Response{Message: "Guest access", Access: "read-only"},
			},
		}

		ctx := context.Background()
		for _, tt := range tests {
			result, err := branch.Run(ctx, tt.user)
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
			if result != tt.want {
				t.Errorf("Run() got = %v, want %v", result, tt.want)
			}
		}
	})

	t.Run("A/B testing", func(t *testing.T) {
		// Type: Branch[int, string] - user segmentation for experiments
		branch, err := NewBranch(BranchConfig[int, string]{
			Branches: map[string]func(context.Context, int) (string, error){
				"variant_a": func(ctx context.Context, userID int) (string, error) {
					return "Variant A: New UI", nil
				},
				"variant_b": func(ctx context.Context, userID int) (string, error) {
					return "Variant B: Old UI", nil
				},
			},
			BranchResolver: func(ctx context.Context, userID int) string {
				// Simple hash-based routing
				if userID%2 == 0 {
					return "variant_a"
				}
				return "variant_b"
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.Background()

		result1, _ := branch.Run(ctx, 100) // Even
		if result1 != "Variant A: New UI" {
			t.Errorf("Even user got %v, want Variant A", result1)
		}

		result2, _ := branch.Run(ctx, 101) // Odd
		if result2 != "Variant B: Old UI" {
			t.Errorf("Odd user got %v, want Variant B", result2)
		}
	})

	t.Run("multi-region routing", func(t *testing.T) {
		// Type: Branch[Request, string] - geographic request routing
		type Request struct {
			Region string
			Data   string
		}

		branch, err := NewBranch(BranchConfig[Request, string]{
			Branches: map[string]func(context.Context, Request) (string, error){
				"us-east": func(ctx context.Context, req Request) (string, error) {
					return "Processed in US-East: " + req.Data, nil
				},
				"eu-west": func(ctx context.Context, req Request) (string, error) {
					return "Processed in EU-West: " + req.Data, nil
				},
				"ap-south": func(ctx context.Context, req Request) (string, error) {
					return "Processed in AP-South: " + req.Data, nil
				},
			},
			BranchResolver: func(ctx context.Context, req Request) string {
				return req.Region
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.Background()
		result, err := branch.Run(ctx, Request{Region: "eu-west", Data: "test"})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "Processed in EU-West: test" {
			t.Errorf("Run() got = %v", result)
		}
	})

	t.Run("content type routing", func(t *testing.T) {
		// Type: Branch[File, string] - file type processing dispatch
		type File struct {
			Name string
			Type string
		}

		branch, err := NewBranch(BranchConfig[File, string]{
			Branches: map[string]func(context.Context, File) (string, error){
				"image": func(ctx context.Context, f File) (string, error) {
					return "Image processing: " + f.Name, nil
				},
				"video": func(ctx context.Context, f File) (string, error) {
					return "Video processing: " + f.Name, nil
				},
				"document": func(ctx context.Context, f File) (string, error) {
					return "Document processing: " + f.Name, nil
				},
			},
			BranchResolver: func(ctx context.Context, f File) string {
				return f.Type
			},
		})
		if err != nil {
			t.Fatalf("NewBranch() error = %v", err)
		}

		ctx := context.Background()
		result, err := branch.Run(ctx, File{Name: "photo.jpg", Type: "image"})
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "Image processing: photo.jpg" {
			t.Errorf("Run() got = %v", result)
		}
	})
}

// TestBranch_MapCloning tests that branch maps are properly cloned.
// Type: Branch[int, string] - validates immutability and isolation
func TestBranch_MapCloning(t *testing.T) {
	originalBranches := map[string]func(context.Context, int) (string, error){
		"branch_a": func(ctx context.Context, i int) (string, error) {
			return "A", nil
		},
	}

	config := BranchConfig[int, string]{
		Branches: originalBranches,
		BranchResolver: func(ctx context.Context, i int) string {
			return "branch_a"
		},
	}

	branch, err := NewBranch(config)
	if err != nil {
		t.Fatalf("NewBranch() error = %v", err)
	}

	// Modify original map
	originalBranches["branch_b"] = func(ctx context.Context, i int) (string, error) {
		return "B", nil
	}

	// Branch should still only have branch_a
	if len(branch.branches) != 1 {
		t.Errorf("Branch map was not properly cloned, got %d branches, want 1", len(branch.branches))
	}

	if _, exists := branch.branches["branch_b"]; exists {
		t.Errorf("Branch map was affected by external modification")
	}
}
