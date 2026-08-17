package toolset

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	toolcontract "github.com/Tangerg/lynx/tool"
)

type roleGoalStub struct{}

type rolePlanStore struct{}

func (rolePlanStore) Replace(_ context.Context, _ string, steps []plan.Step) (plan.State, error) {
	return (plan.State{}).Replace(steps, time.Now())
}
func (rolePlanStore) State(context.Context, string) (plan.State, error) {
	return (plan.State{}).Replace([]plan.Step{{Description: "implement", Status: plan.StatusPending}}, time.Now())
}

func (roleGoalStub) Current(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (roleGoalStub) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

func TestPlanModeToolsAreRootOnly(t *testing.T) {
	policy, err := approvals.NewRuntimePolicy(approval.ModeBalanced, nil, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(),
		DefaultCWD: t.TempDir(),
		UserHome:   t.TempDir(),
		PlanMode:   policy,
		Plan:       rolePlanStore{},
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupt.Resolution, error) {
			return interrupt.Resolution{}, nil
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
	delegated, err := built.Resolver.Manifest(t.Context(), domaintool.GroupDelegated)
	if err != nil {
		t.Fatalf("Manifest(delegated): %v", err)
	}
	names := definitionNames(manifestTools(delegated))
	if !names["ask_user"] {
		t.Fatalf("delegated tools = %v, want ask_user", names)
	}
	if names["delegate_task"] || names["list_schedules"] || names["create_schedule"] || names["delete_schedule"] {
		t.Fatalf("delegated tools = %v; delegation belongs to Agent Framework Definition, not the Tool manifest", names)
	}
	if names["enter_plan_mode"] || names["exit_plan_mode"] || names["set_plan"] {
		t.Fatalf("delegated tools = %v; Plan control belongs only to the root Agent", names)
	}

	root, err := built.Resolver.Manifest(t.Context(), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest(root): %v", err)
	}
	foundPlan := false
	foundEnter := false
	foundExit := false
	for _, candidate := range manifestTools(root) {
		foundPlan = foundPlan || candidate.Definition().Name == "set_plan"
		foundEnter = foundEnter || candidate.Definition().Name == "enter_plan_mode"
		foundExit = foundExit || candidate.Definition().Name == "exit_plan_mode"
	}
	if !foundPlan || !foundEnter || !foundExit {
		t.Fatalf("root tools: set_plan=%v enter=%v exit=%v", foundPlan, foundEnter, foundExit)
	}
}

func TestGoalToolsAreRootOnlyAndOutcomeRequiresGoalRunProvenance(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(),
		DefaultCWD: t.TempDir(), UserHome: t.TempDir(),
		// Deliberately inactive: manifest membership must remain tied to the Run's
		// frozen incarnation even when mutable Goal state pauses for HITL.
		GoalReader:   roleGoalStub{},
		GoalReporter: roleGoalStub{},
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
	goalRunContext := executionctx.WithScope(t.Context(), runs.ExecutionScope{
		SessionID: "session-goal", GoalIncarnationID: "incarnation-1",
	})

	for _, tc := range []struct {
		name string
		ctx  context.Context
		role string
		want map[string]bool
	}{
		{
			name: "goal-owned root", ctx: goalRunContext, role: domaintool.GroupRoot,
			want: map[string]bool{"create_goal": true, "get_goal": true, "report_goal_outcome": true},
		},
		{
			name: "ordinary root", ctx: t.Context(), role: domaintool.GroupRoot,
			want: map[string]bool{"create_goal": true, "get_goal": true},
		},
		{name: "goal-owned delegate", ctx: goalRunContext, role: domaintool.GroupDelegated, want: map[string]bool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := built.Resolver.Manifest(tc.ctx, tc.role)
			if err != nil {
				t.Fatalf("Manifest(%s): %v", tc.role, err)
			}
			names := definitionNames(manifestTools(manifest))
			for _, name := range []string{"create_goal", "get_goal", "report_goal_outcome"} {
				if names[name] != tc.want[name] {
					t.Errorf("role %s tool %s present=%v, want %v", tc.role, name, names[name], tc.want[name])
				}
			}
		})
	}
}

func TestProposeSkillIsRootOnlyAndDeferred(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(),
		DefaultCWD:     t.TempDir(),
		UserHome:       t.TempDir(),
		SkillProposals: allWiredSkillProposals{},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	for _, tc := range []struct {
		role string
		want bool
	}{
		{role: domaintool.GroupRoot, want: true},
		{role: domaintool.GroupDelegated, want: false},
	} {
		manifest, err := built.Resolver.Manifest(t.Context(), tc.role)
		if err != nil {
			t.Fatalf("Manifest(%s): %v", tc.role, err)
		}
		if got := definitionNames(manifestTools(manifest))["propose_skill"]; got != tc.want {
			t.Errorf("role %s propose_skill present=%v, want %v", tc.role, got, tc.want)
		}
		for _, executable := range manifest.Visible {
			if executable.Definition().Name == "propose_skill" {
				t.Errorf("role %s advertised deferred propose_skill", tc.role)
			}
		}
	}
}

func TestResolverAcceptsOnlyCanonicalRoleNames(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	for _, role := range []string{"root", "delegated"} {
		if _, err := built.Resolver.Manifest(t.Context(), role); err != nil {
			t.Errorf("Manifest(%q): %v", role, err)
		}
	}
	for _, obsolete := range []string{"coding", "subtask"} {
		if _, err := built.Resolver.Manifest(t.Context(), obsolete); err == nil {
			t.Errorf("Manifest accepted obsolete role %q", obsolete)
		}
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
