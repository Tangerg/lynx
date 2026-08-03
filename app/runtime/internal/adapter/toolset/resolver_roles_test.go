package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	toolcontract "github.com/Tangerg/lynx/tool"
)

type availabilityIndex struct {
	available bool
	err       error
}

type failingGoalState struct{ err error }
type roleGoalState struct{ active bool }

type rolePlanStore struct{}

func (rolePlanStore) Replace(context.Context, string, []plan.Step) error { return nil }
func (rolePlanStore) List(context.Context, string) ([]plan.Step, error) {
	return []plan.Step{{Description: "implement", Status: plan.StatusPending}}, nil
}

func (failingGoalState) Get(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (s failingGoalState) Active(context.Context, string) (bool, error) { return false, s.err }
func (failingGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

func (roleGoalState) Get(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (s roleGoalState) Active(context.Context, string) (bool, error) { return s.active, nil }
func (roleGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

func (i availabilityIndex) Available(context.Context) (bool, error) {
	return i.available, i.err
}

func (availabilityIndex) Search(context.Context, string, string, int) ([]codebaseindex.Hit, error) {
	return nil, nil
}

func TestPlanModeToolsAreRootOnly(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		PlanMode: policy,
		Plan:     rolePlanStore{},
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
	taskTool, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{Name: "delegate_task", Description: "Delegate a bounded child task."},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("build task tool: %v", err)
	}
	built.Resolver.UseDelegationTool(taskTool)

	group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupSubtask)
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
	if !names["ask_user"] {
		t.Fatalf("subtask tools = %v, want ask_user", names)
	}
	if !names["delegate_task"] || names["list_schedules"] || names["create_schedule"] || names["delete_schedule"] {
		t.Fatalf("subtask tools = %v, want bounded delegation without root-only schedule tools", names)
	}
	if names["enter_plan_mode"] || names["exit_plan_mode"] || names["set_plan"] {
		t.Fatalf("subtask tools = %v; Plan control belongs only to the root Agent", names)
	}

	coding, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	codingTools, err := coding.Tools(t.Context())
	if err != nil {
		t.Fatalf("coding Tools: %v", err)
	}
	foundPlan := false
	foundEnter := false
	foundExit := false
	for _, candidate := range codingTools {
		foundPlan = foundPlan || candidate.Definition().Name == "set_plan"
		foundEnter = foundEnter || candidate.Definition().Name == "enter_plan_mode"
		foundExit = foundExit || candidate.Definition().Name == "exit_plan_mode"
	}
	if !foundPlan || !foundEnter || !foundExit {
		t.Fatalf("coding tools: set_plan=%v enter=%v exit=%v", foundPlan, foundEnter, foundExit)
	}
}

func TestGoalToolsAreRootOnlyAndOutcomeRequiresActiveGoal(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{
		Workdir: t.TempDir(),
		Goals:   roleGoalState{active: true},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	create, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{Name: "create_goal", Description: "Create an autonomous Goal."},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("create_goal: %v", err)
	}
	built.Resolver.UseCreateGoalTool(create)

	for _, tc := range []struct {
		role string
		want map[string]bool
	}{
		{role: domaintool.GroupCoding, want: map[string]bool{"create_goal": true, "get_goal": true, "report_goal_outcome": true}},
		{role: domaintool.GroupSubtask, want: map[string]bool{}},
	} {
		group, ok, err := built.Resolver.Resolve(t.Context(), tc.role)
		if err != nil || !ok {
			t.Fatalf("Resolve(%s) = %v, %v", tc.role, ok, err)
		}
		resolved, err := group.Tools(t.Context())
		if err != nil {
			t.Fatalf("Tools(%s): %v", tc.role, err)
		}
		names := toolNameSet(resolved)
		for _, name := range []string{"create_goal", "get_goal", "report_goal_outcome"} {
			if names[name] != tc.want[name] {
				t.Errorf("role %s tool %s present=%v, want %v", tc.role, name, names[name], tc.want[name])
			}
		}
	}

	inactive, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir(), Goals: roleGoalState{}})
	if err != nil {
		t.Fatalf("Build(inactive): %v", err)
	}
	closeBuiltToolset(t, inactive)
	group, _, err := inactive.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	names := toolNameSet(resolved)
	if !names["get_goal"] || names["report_goal_outcome"] {
		t.Fatalf("inactive Goal tools = %v", names)
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

		group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
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

		group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
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

	group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
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
