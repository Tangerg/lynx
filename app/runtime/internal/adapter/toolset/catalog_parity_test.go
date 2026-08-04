package toolset

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	goaladapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
	create, err := goaladapter.NewCreate(allWiredGoalStarter{})
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

type allWiredToolResults struct{}

func (allWiredToolResults) Fetch(context.Context, string, resultoffload.ID) (string, bool, error) {
	return "", false, nil
}

type allWiredMemorySearch struct{}

func (allWiredMemorySearch) Search(context.Context, agentmemory.Scope, string, string, int) ([]agentmemory.Item, error) {
	return nil, nil
}

type allWiredSessionSearch struct{}

func (allWiredSessionSearch) SearchTranscript(context.Context, string, int) ([]transcript.SearchHit, error) {
	return nil, nil
}

type allWiredSkillProposals struct{}

func (allWiredSkillProposals) SubmitSkillProposal(_ context.Context, _ string, proposal skills.Proposal) (skills.ProposalRef, error) {
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), nil
}

func toolNameSet(ts []toolcontract.Tool) map[string]bool {
	names := make(map[string]bool, len(ts))
	for _, t := range ts {
		names[t.Definition().Name] = true
	}
	return names
}

func assertBuiltInToolContract(t *testing.T, candidate toolcontract.Tool) {
	t.Helper()
	definition := candidate.Definition()
	if err := definition.Validate(); err != nil {
		t.Errorf("built-in tool %q has an invalid definition: %v", definition.Name, err)
		return
	}
	var schema struct {
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Errorf("built-in tool %q has invalid input schema JSON: %v", definition.Name, err)
		return
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Errorf("built-in tool %q does not explicitly reject unknown input fields", definition.Name)
	}
	modelText := strings.ToLower(definition.Description + " " + string(definition.InputSchema))
	terms := strings.FieldsFunc(modelText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, forbidden := range []string{"ui", "frontend", "runtime", "client", "operator", "chip", "button"} {
		if slices.Contains(terms, forbidden) {
			t.Errorf("built-in tool %q leaks implementation term %q into its model contract", definition.Name, forbidden)
		}
	}
}

func TestRootResolverIncludesConfiguredConditionalTools(t *testing.T) {
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

	group, ok, err := built.Resolver.Resolve(t.Context(), tool.GroupRoot)
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
			t.Errorf("configured root tools missing %q: %v", want, names)
		}
	}
}

// TestSafetyTableMatchesBuiltInTools is the two-way completeness guard for the
// built-in name→safety-class table.
//
// That table is the single source of truth for two consumers — the tools.list
// wire metadata and the approval gate — and it is keyed by NAME, so a name no
// tool answers to is a safety policy for something nobody can call. It reads as a
// capability the runtime has: a dead key claims a tool exists, while an omitted
// key silently classifies a known built-in as arbitrary Exec. The names span app
// adapters and SDK tool modules, so grepping one package cannot enforce either
// direction.
//
// The resolver is built with every optional subsystem wired, because a name is
// only unreachable if NO configuration reaches it.
func TestSafetyTableMatchesBuiltInTools(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:                t.TempDir(),
		SkillsUserDir:          t.TempDir(), // backs skill
		PlanMode:               policy,
		Plan:                   rolePlanStore{},
		Goals:                  activeGoalState{},
		Schedules:              allWiredSchedules{},   // backs schedule
		ToolResults:            allWiredToolResults{}, // backs read_tool_result
		MemorySearch:           allWiredMemorySearch{},
		SessionSearch:          allWiredSessionSearch{},
		SkillProposalSubmitter: allWiredSkillProposals{},
		Online: OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"example.com"}, // backs http_request
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

	group, ok, err := built.Resolver.Resolve(t.Context(), tool.GroupRoot)
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
	checkedContracts := make(map[string]bool)
	for _, profile := range [][]toolcontract.Tool{resolved, patchTools} {
		for _, candidate := range profile {
			name := candidate.Definition().Name
			if checkedContracts[name] {
				continue
			}
			checkedContracts[name] = true
			assertBuiltInToolContract(t, candidate)
		}
	}
	// The engine injects delegate_task only after it deploys the child Agent.
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
	classified := make(map[string]bool)
	for _, name := range tool.ClassifiedToolNames() {
		classified[name] = true
	}
	var unclassified []string
	for name := range existing {
		if !classified[name] {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		slices.Sort(unclassified)
		t.Errorf("built-in tools %v rely on the unknown-tool Exec fallback — classify each one explicitly", unclassified)
	}
}
