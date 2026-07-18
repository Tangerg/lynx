package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

type availabilityIndex struct {
	available bool
	err       error
}

func (i availabilityIndex) Available(context.Context) (bool, error) {
	return i.available, i.err
}

func (availabilityIndex) Search(context.Context, string, string, int) ([]codebaseindex.Hit, error) {
	return nil, nil
}

func TestSubtaskRoleCanAskAndExitPlanWithoutDelegating(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		Approval: policy,
		Interrupt: func(context.Context, string, any) (interrupts.Resolution, error) {
			return interrupts.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		for _, close := range built.Closers {
			_ = close()
		}
	})

	group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleSubtask})
	if err != nil || !ok {
		t.Fatalf("Resolve(subtask) = %v, %v", ok, err)
	}
	tools, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Definition().Name] = true
	}
	if !names["ask_user"] || !names["exit_plan_mode"] {
		t.Fatalf("subtask tools = %v, want ask_user and exit_plan_mode", names)
	}
	if names["task"] || names["schedule"] {
		t.Fatalf("subtask tools = %v, task/schedule must stay root-only", names)
	}
}

func TestToolGroupDistinguishesUnavailableCodebaseFromResolverFailure(t *testing.T) {
	t.Run("unconfigured model omits tool", func(t *testing.T) {
		built, err := Build(t.Context(), BuildConfig{
			Workdir:       t.TempDir(),
			CodebaseIndex: availabilityIndex{},
		})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		closeBuiltToolset(t, built)

		group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleCoding})
		if err != nil || !ok {
			t.Fatalf("Resolve(coding) = %v, %v", ok, err)
		}
		resolved, err := group.Tools(t.Context())
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		for _, candidate := range resolved {
			if candidate.Definition().Name == "codebase_search" {
				t.Fatal("codebase_search offered without an embedding model")
			}
		}
	})

	t.Run("resolver failure is preserved", func(t *testing.T) {
		wantErr := errors.New("provider store unavailable")
		built, err := Build(t.Context(), BuildConfig{
			Workdir: t.TempDir(),
			CodebaseIndex: availabilityIndex{
				err: wantErr,
			},
		})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		closeBuiltToolset(t, built)

		group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleCoding})
		if err != nil || !ok {
			t.Fatalf("Resolve(coding) = %v, %v", ok, err)
		}
		if _, err := group.Tools(t.Context()); !errors.Is(err, wantErr) {
			t.Fatalf("Tools error = %v, want %v", err, wantErr)
		}
	})
}

func closeBuiltToolset(t *testing.T, built Built) {
	t.Helper()
	t.Cleanup(func() {
		for _, close := range built.Closers {
			if err := close(); err != nil {
				t.Errorf("close toolset: %v", err)
			}
		}
	})
}
