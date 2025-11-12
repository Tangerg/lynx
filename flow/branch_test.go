package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestBranchConfig_validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *BranchConfig

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if err.Error() != "branch config cannot be nil" {
			t.Errorf("expected 'branch config cannot be nil' error, got %v", err)
		}
	})

	t.Run("nil node", func(t *testing.T) {
		cfg := &BranchConfig{
			Node: nil,
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil node, got nil")
		}

		if err.Error() != "branch node cannot be nil" {
			t.Errorf("expected 'branch node cannot be nil' error, got %v", err)
		}
	})

	t.Run("valid config with branches", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "branch1", nil
			},
			Branches: map[string]Node[any, any]{
				"branch1": mockNode,
			},
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestNewBranch(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if branch == nil {
			t.Error("expected non-nil branch")
		}

		if branch.node == nil {
			t.Error("expected non-nil node in branch")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &BranchConfig{
			Node: nil,
		}

		branch, err := NewBranch(cfg)
		if err == nil {
			t.Error("expected error for invalid config, got nil")
		}

		if branch != nil {
			t.Error("expected nil branch for invalid config")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		branch, err := NewBranch(nil)
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if branch != nil {
			t.Error("expected nil branch for nil config")
		}
	})

	t.Run("config with branches", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "test", nil
			},
			Branches: map[string]Node[any, any]{
				"test": mockNode,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(branch.branches) != 1 {
			t.Errorf("expected 1 branch, got %d", len(branch.branches))
		}
	})
}

func TestBranch_resolveBranch(t *testing.T) {
	t.Run("successful branch resolution", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		branch := &Branch{
			node: mockNode,
			branchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "success", nil
			},
			branches: map[string]Node[any, any]{
				"success": mockNode,
			},
		}

		node, err := branch.resolveBranch(context.Background(), "input", "output")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if node == nil {
			t.Error("expected non-nil node")
		}
	})

	t.Run("branch resolver returns error", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		expectedErr := errors.New("resolver error")
		branch := &Branch{
			node: mockNode,
			branchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "", expectedErr
			},
			branches: map[string]Node[any, any]{
				"success": mockNode,
			},
		}

		_, err := branch.resolveBranch(context.Background(), "input", "output")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("branch not found", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		branch := &Branch{
			node: mockNode,
			branchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "nonexistent", nil
			},
			branches: map[string]Node[any, any]{
				"branch1": mockNode,
				"branch2": mockNode,
			},
		}

		_, err := branch.resolveBranch(context.Background(), "input", "output")
		if err == nil {
			t.Error("expected error for nonexistent branch, got nil")
		}

		expectedMsg := "branch 'nonexistent' not found"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("empty branches map", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		branch := &Branch{
			node: mockNode,
			branchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "any", nil
			},
			branches: map[string]Node[any, any]{},
		}

		_, err := branch.resolveBranch(context.Background(), "input", "output")
		if err == nil {
			t.Error("expected error for empty branches, got nil")
		}
	})
}

func TestBranch_Run(t *testing.T) {
	t.Run("main node only without branches", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("processed: %v", input), nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %v", expected, result)
		}
	})

	t.Run("main node with nil resolver", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("processed: %v", input), nil
		})

		branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("branch: %v", input), nil
		})

		cfg := &BranchConfig{
			Node:           mockNode,
			BranchResolver: nil,
			Branches: map[string]Node[any, any]{
				"test": branchNode,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %v", expected, result)
		}
	})

	t.Run("main node with empty branches", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("processed: %v", input), nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "test", nil
			},
			Branches: map[string]Node[any, any]{},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), "test")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "processed: test"
		if result != expected {
			t.Errorf("expected %q, got %v", expected, result)
		}
	})

	t.Run("successful branching", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(int) * 2, nil
		})

		successBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("success: %d", input), nil
		})

		failureBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return fmt.Sprintf("failure: %d", input), nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				if output.(int) > 10 {
					return "success", nil
				}
				return "failure", nil
			},
			Branches: map[string]Node[any, any]{
				"success": successBranch,
				"failure": failureBranch,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), 10)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "success: 20"
		if result != expected {
			t.Errorf("expected %q, got %v", expected, result)
		}
	})

	t.Run("branch based on input and output", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input.(string) + "-processed", nil
		})

		branch1 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch1: " + input.(string), nil
		})

		branch2 := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return "branch2: " + input.(string), nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				if input.(string) == "test1" {
					return "branch1", nil
				}
				return "branch2", nil
			},
			Branches: map[string]Node[any, any]{
				"branch1": branch1,
				"branch2": branch2,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), "test1")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := "branch1: test1-processed"
		if result != expected {
			t.Errorf("expected %q, got %v", expected, result)
		}
	})

	t.Run("main node returns error", func(t *testing.T) {
		expectedErr := errors.New("main node error")
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return nil, expectedErr
		})

		cfg := &BranchConfig{
			Node: mockNode,
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		_, err = branch.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("branch resolver returns error", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		expectedErr := errors.New("resolver error")
		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "", expectedErr
			},
			Branches: map[string]Node[any, any]{
				"test": mockNode,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		_, err = branch.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("branch not found error", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "nonexistent", nil
			},
			Branches: map[string]Node[any, any]{
				"branch1": mockNode,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		_, err = branch.Run(context.Background(), "test")
		if err == nil {
			t.Error("expected error for nonexistent branch, got nil")
		}
	})

	t.Run("branch node returns error", func(t *testing.T) {
		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return input, nil
		})

		expectedErr := errors.New("branch node error")
		branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return nil, expectedErr
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				return "error-branch", nil
			},
			Branches: map[string]Node[any, any]{
				"error-branch": branchNode,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		_, err = branch.Run(context.Background(), "test")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("complex branching scenario", func(t *testing.T) {
		type Result struct {
			Value  int
			Status string
		}

		mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			value := input.(int)
			return Result{Value: value * 2, Status: "pending"}, nil
		})

		lowBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			r := input.(Result)
			r.Status = "low"
			return r, nil
		})

		highBranch := Processor[any, any](func(ctx context.Context, input any) (any, error) {
			r := input.(Result)
			r.Status = "high"
			return r, nil
		})

		cfg := &BranchConfig{
			Node: mockNode,
			BranchResolver: func(ctx context.Context, input, output any) (string, error) {
				r := output.(Result)
				if r.Value < 50 {
					return "low", nil
				}
				return "high", nil
			},
			Branches: map[string]Node[any, any]{
				"low":  lowBranch,
				"high": highBranch,
			},
		}

		branch, err := NewBranch(cfg)
		if err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		result, err := branch.Run(context.Background(), 20)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		r := result.(Result)
		if r.Value != 40 {
			t.Errorf("expected value 40, got %d", r.Value)
		}
		if r.Status != "low" {
			t.Errorf("expected status 'low', got %q", r.Status)
		}
	})
}

func BenchmarkBranch_Run_NoBranching(b *testing.B) {
	mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})

	cfg := &BranchConfig{
		Node: mockNode,
	}

	branch, _ := NewBranch(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = branch.Run(ctx, 10)
	}
}

func BenchmarkBranch_Run_WithBranching(b *testing.B) {
	mockNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})

	branchNode := Processor[any, any](func(ctx context.Context, input any) (any, error) {
		return input.(int) + 10, nil
	})

	cfg := &BranchConfig{
		Node: mockNode,
		BranchResolver: func(ctx context.Context, input, output any) (string, error) {
			return "branch1", nil
		},
		Branches: map[string]Node[any, any]{
			"branch1": branchNode,
		},
	}

	branch, _ := NewBranch(cfg)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = branch.Run(ctx, 10)
	}
}
