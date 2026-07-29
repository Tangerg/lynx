package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// emptyGoalState is enough to make goaltool.New return a non-nil update_goal
// tool (Goal mode wired) without an active goal.
type emptyGoalState struct{}

func (emptyGoalState) Active(context.Context, string) (bool, error) { return false, nil }
func (emptyGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

// Every conditional tool's port, wired with the smallest thing that makes the
// tool exist. They are not exercised: the guard below asks whether a classified
// name can be reached at all, and a nil port answers "no" for a reason that has
// nothing to do with drift.
type allWiredSchedules struct{}

func (allWiredSchedules) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (allWiredSchedules) Create(context.Context, scheduleapp.CreateCommand) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (allWiredSchedules) UpdateLatest(context.Context, string, schedule.Patch) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (allWiredSchedules) Delete(context.Context, string) error { return nil }

type allWiredCodebaseIndex struct{}

func (allWiredCodebaseIndex) Search(context.Context, string, string, int) ([]codebaseindex.Hit, error) {
	return nil, nil
}
func (allWiredCodebaseIndex) Available(context.Context) (bool, error) { return true, nil }

type allWiredSkillAuthoring struct{}

func (allWiredSkillAuthoring) Enabled() bool { return true }
func (allWiredSkillAuthoring) SaveDraft(context.Context, skills.Draft) (skills.DraftHandle, error) {
	return skills.DraftHandle{}, nil
}
func (allWiredSkillAuthoring) Promote(context.Context, skills.DraftHandle) error      { return nil }
func (allWiredSkillAuthoring) DiscardDraft(context.Context, skills.DraftHandle) error { return nil }

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

	catalog := toolNameSet(built.Resolver.toolsFor(t.Context()))
	// The two tools the catalog historically dropped.
	for _, want := range []string{"exit_plan_mode", "update_goal"} {
		if !catalog[want] {
			t.Fatalf("tools.list catalog missing %q: %v", want, catalog)
		}
	}

	group, ok, err := built.Resolver.Resolve(t.Context(), toolport.ToolRoleCoding)
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

// TestSafetyTableNamesOnlyToolsThatExist is the completeness guard for the
// name→safety-class table.
//
// That table is the single source of truth for two consumers — the tools.list
// wire metadata and the approval gate — and it is keyed by NAME, so a name no
// tool answers to is a safety policy for something nobody can call. It reads as a
// capability the runtime has: someone auditing what the agent may do sees a
// classified write tool that does not exist. Nothing checked, and the names come
// from two modules (the app's own tools plus the SDK's fs / shell families), so
// finding out meant grepping both.
//
// The catalog is built with every optional subsystem wired, because a name is
// only unreachable if NO configuration reaches it.
func TestSafetyTableNamesOnlyToolsThatExist(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:         t.TempDir(),
		SkillsGlobalDir: t.TempDir(), // backs skill
		Approval:        policy,
		Goals:           emptyGoalState{},
		Schedules:       allWiredSchedules{},      // backs schedule
		CodebaseIndex:   allWiredCodebaseIndex{},  // backs codebase_search
		SkillAuthoring:  allWiredSkillAuthoring{}, // backs propose_skill
		Online: OnlineConfig{
			HTTPAllowedHosts:    []string{"example.com"},   // backs download
			SourcegraphEndpoint: "https://sourcegraph.com", // backs sourcegraph_search
		},
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
			return interrupts.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	existing := toolNameSet(built.Resolver.toolsFor(t.Context()))
	// `task` is appended by the engine after the catalog is built, so the catalog
	// never contains it — the same exemption the parity guard above makes.
	existing["task"] = true

	var unreachable []string
	for _, name := range tool.ClassifiedToolNames() {
		if !existing[name] {
			unreachable = append(unreachable, name)
		}
	}
	if len(unreachable) > 0 {
		t.Errorf("the safety table classifies %v, which no built tool answers to — "+
			"either the tool is gone (drop the name) or it was never built (wire it)", unreachable)
	}
}
