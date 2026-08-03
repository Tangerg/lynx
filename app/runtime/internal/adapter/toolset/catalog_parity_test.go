package toolset

import (
	"context"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/goaltool"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type activeGoalState struct{}

func (activeGoalState) Get(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (activeGoalState) Active(context.Context, string) (bool, error) { return true, nil }
func (activeGoalState) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

type allWiredGoalStarter struct{}

func (allWiredGoalStarter) Start(context.Context, string, string, modelref.Selection, goal.Budget) (goal.Goal, error) {
	return goal.Goal{}, nil
}

func wireCreateGoal(t *testing.T, resolver *Resolver) {
	t.Helper()
	create, err := goaltool.NewCreate(allWiredGoalStarter{})
	if err != nil {
		t.Fatalf("build create_goal: %v", err)
	}
	resolver.UseCreateGoalTool(create)
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

type allWiredToolResults struct{}

func (allWiredToolResults) Fetch(context.Context, string, resultoffload.ID) (string, bool, error) {
	return "", false, nil
}

func (allWiredSkillAuthoring) Enabled() bool { return true }
func (allWiredSkillAuthoring) SaveDraft(context.Context, skills.Draft) (skills.DraftHandle, error) {
	return skills.DraftHandle{}, nil
}
func (allWiredSkillAuthoring) Promote(context.Context, skills.DraftHandle) error      { return nil }
func (allWiredSkillAuthoring) DiscardDraft(context.Context, skills.DraftHandle) error { return nil }

func toolNameSet(ts []toolcontract.Tool) map[string]bool {
	names := make(map[string]bool, len(ts))
	for _, t := range ts {
		names[t.Definition().Name] = true
	}
	return names
}

func TestCodingResolverIncludesConfiguredConditionalTools(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		PlanMode: policy,
		Plan:     rolePlanStore{},
		Goals:    activeGoalState{},
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
			return interrupts.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	wireCreateGoal(t, built.Resolver)

	group, ok, err := built.Resolver.Resolve(t.Context(), tool.GroupCoding)
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := toolNameSet(resolved)
	for _, want := range []string{"enter_plan_mode", "exit_plan_mode", "create_goal", "get_goal", "report_goal_outcome"} {
		if !names[want] {
			t.Errorf("configured coding tools missing %q: %v", want, names)
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
// The resolver is built with every optional subsystem wired, because a name is
// only unreachable if NO configuration reaches it.
func TestSafetyTableNamesOnlyToolsThatExist(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:         t.TempDir(),
		SkillsGlobalDir: t.TempDir(), // backs skill
		PlanMode:        policy,
		Plan:            rolePlanStore{},
		Goals:           activeGoalState{},
		Schedules:       allWiredSchedules{},      // backs schedule
		CodebaseIndex:   allWiredCodebaseIndex{},  // backs codebase_search
		SkillAuthoring:  allWiredSkillAuthoring{}, // backs propose_skill
		ToolResults:     allWiredToolResults{},    // backs read_tool_result
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
	wireCreateGoal(t, built.Resolver)

	group, ok, err := built.Resolver.Resolve(t.Context(), tool.GroupCoding)
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	existing := toolNameSet(resolved)
	// A Run receives exactly one mutation vocabulary. Union the other supported
	// profile before checking the global safety table, which necessarily covers
	// both vocabularies.
	patchModel, err := modelref.New("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	patchTools, err := group.Tools(executionctx.WithModelSelection(t.Context(), patchModel))
	if err != nil {
		t.Fatalf("Tools(apply_patch profile): %v", err)
	}
	for name := range toolNameSet(patchTools) {
		existing[name] = true
	}
	// The engine injects task only after it deploys the child Agent.
	existing["delegate_task"] = true

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
