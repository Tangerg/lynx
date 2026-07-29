package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/tools"
)

type availabilityIndex struct {
	available bool
	err       error
}

type failingGoalState struct{ err error }

func (s failingGoalState) Active(context.Context, string) (bool, error) { return false, s.err }
func (failingGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

func (i availabilityIndex) Available(context.Context) (bool, error) {
	return i.available, i.err
}

func (availabilityIndex) Search(context.Context, string, string, int) ([]codebaseindex.Hit, error) {
	return nil, nil
}

func TestSubtaskRoleCanAskExitPlanAndDelegateWithoutRootTools(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		Approval: policy,
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
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
	taskTool, err := tools.New(
		tools.Config{Name: "task", Description: "Delegate a bounded child task."},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("build task tool: %v", err)
	}
	built.Resolver.UseTaskTool(taskTool)

	group, ok, err := built.Resolver.Resolve(t.Context(), toolport.ToolRoleSubtask)
	if err != nil || !ok {
		t.Fatalf("Resolve(subtask) = %v, %v", ok, err)
	}
	resolvedTools, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := make(map[string]bool, len(resolvedTools))
	for _, tool := range resolvedTools {
		names[tool.Definition().Name] = true
	}
	if !names["ask_user"] || !names["exit_plan_mode"] {
		t.Fatalf("subtask tools = %v, want ask_user and exit_plan_mode", names)
	}
	if !names["task"] || names["schedule"] {
		t.Fatalf("subtask tools = %v, want bounded task delegation without root-only schedule", names)
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

		group, ok, err := built.Resolver.Resolve(t.Context(), toolport.ToolRoleCoding)
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

		group, ok, err := built.Resolver.Resolve(t.Context(), toolport.ToolRoleCoding)
		if err != nil || !ok {
			t.Fatalf("Resolve(coding) = %v, %v", ok, err)
		}
		if _, err := group.Tools(t.Context()); !errors.Is(err, wantErr) {
			t.Fatalf("Tools error = %v, want %v", err, wantErr)
		}
	})
}

func TestToolGroupPreservesActiveGoalLookupFailure(t *testing.T) {
	wantErr := errors.New("goal store unavailable")
	built, err := Build(t.Context(), BuildConfig{
		Workdir: t.TempDir(),
		Goals:   failingGoalState{err: wantErr},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	group, ok, err := built.Resolver.Resolve(t.Context(), toolport.ToolRoleCoding)
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	if _, err := group.Tools(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("Tools error = %v, want %v", err, wantErr)
	}
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
