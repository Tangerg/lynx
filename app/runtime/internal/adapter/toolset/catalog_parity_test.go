package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// emptyGoalState is enough to make goaltool.New return a non-nil update_goal
// tool (Goal mode wired) without an active goal.
type emptyGoalState struct{}

func (emptyGoalState) Active(context.Context, string) (bool, error) { return false, nil }
func (emptyGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

func toolNameSet(ts []tools.Tool) map[string]bool {
	names := make(map[string]bool, len(ts))
	for _, t := range ts {
		names[t.Definition().Name] = true
	}
	return names
}

// TestCatalogCoversPerTurnCodingTools is the tools.list parity guard: the
// direct catalog (tools.list) and the per-turn coding manifest have intentionally
// different gates and drifted once (exit_plan_mode / update_goal were dropped
// from the catalog). The catalog is the "possibly exists" tier — it must
// cover every tool the coding turn can offer EXCEPT `task`, which the engine
// appends after the catalog is built. Raw MCP tools vs. search_tools stay a
// deliberate difference and are covered by the raw append in Build.
func TestCatalogCoversPerTurnCodingTools(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		Approval: policy,           // backs exit_plan_mode
		Goals:    emptyGoalState{}, // backs update_goal (Goal mode wired)
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
			return interrupts.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	catalog := toolNameSet(built.Resolver.Tools())
	// The two tools the catalog historically dropped.
	for _, want := range []string{"exit_plan_mode", "update_goal"} {
		if !catalog[want] {
			t.Fatalf("tools.list catalog missing %q: %v", want, catalog)
		}
	}

	group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleCoding})
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	perTurn, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	for _, tl := range perTurn {
		name := tl.Definition().Name
		if name == "task" { // engine-appended after the catalog is built
			continue
		}
		if !catalog[name] {
			t.Errorf("per-turn coding tool %q is absent from the tools.list catalog (drift)", name)
		}
	}
}
