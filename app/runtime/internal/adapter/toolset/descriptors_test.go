package toolset

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type activeGoalStub struct{}

func (activeGoalStub) Current(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (activeGoalStub) Active(context.Context, string) (bool, error) { return true, nil }
func (activeGoalStub) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

type allWiredGoalStarter struct{}

func (allWiredGoalStarter) Start(context.Context, string, string, modelref.Selection, goal.Budget, run.Capabilities) (goal.Goal, error) {
	return goal.Goal{}, nil
}

func wireCreateGoal(t *testing.T, resolver *Resolver) {
	t.Helper()
	create, err := builtin.NewCreate(allWiredGoalStarter{})
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
func (allWiredSchedules) Delete(context.Context, string) error { return nil }

type allWiredToolResults struct{}

func (allWiredToolResults) Fetch(context.Context, string, resultoffload.ID) (string, bool, error) {
	return "", false, nil
}

type allWiredAgentMemorySearch struct{}

func (allWiredAgentMemorySearch) Search(context.Context, agentmemory.Scope, string, string, int) ([]agentmemory.Item, error) {
	return nil, nil
}

type allWiredConversationSearch struct{}

func (allWiredConversationSearch) SearchTranscript(context.Context, string, int) ([]transcript.SearchHit, error) {
	return nil, nil
}

type allWiredSkillProposals struct{}

func (allWiredSkillProposals) SubmitProposal(_ context.Context, _ string, proposal skills.Proposal) (skills.ProposalRef, error) {
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), nil
}

func definitionNames(ts []toolcontract.Tool) map[string]bool {
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
	policy, err := approvals.NewRuntimePolicy(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		DefaultCWD:   t.TempDir(),
		UserHome:     t.TempDir(),
		PlanMode:     policy,
		Plan:         rolePlanStore{},
		GoalReader:   activeGoalStub{},
		GoalReporter: activeGoalStub{},
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupt.Resolution, error) {
			return interrupt.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	wireCreateGoal(t, built.Resolver)

	manifest, err := built.Resolver.Manifest(t.Context(), tool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	names := definitionNames(manifestTools(manifest))
	for _, want := range []string{"enter_plan_mode", "exit_plan_mode", "create_goal", "get_goal", "report_goal_outcome"} {
		if !names[want] {
			t.Errorf("configured root tools missing %q: %v", want, names)
		}
	}
}

// TestDescriptorCatalogMatchesBuiltInTools is the two-way completeness guard for the
// built-in descriptor catalog.
//
// The catalog is the single cross-cutting source for safety, policy exceptions,
// activity, presentation, and outcome projection. A dead key claims a tool
// exists, while an omitted key silently sends a known built-in through unknown-
// tool fallbacks. Definitions span Runtime adapters and SDK tool modules, so
// grepping one package cannot enforce either direction.
//
// The resolver is built with every optional subsystem wired, because a name is
// only unreachable if NO configuration reaches it.
func TestDescriptorCatalogMatchesBuiltInTools(t *testing.T) {
	policy, err := approvals.NewRuntimePolicy(approval.ModeBalanced, nil, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		DefaultCWD:         t.TempDir(),
		UserHome:           t.TempDir(),
		SkillsUserDir:      t.TempDir(), // backs skill
		PlanMode:           policy,
		Plan:               rolePlanStore{},
		GoalReader:         activeGoalStub{},
		GoalReporter:       activeGoalStub{},
		Schedules:          allWiredSchedules{},   // backs schedule
		ToolResults:        allWiredToolResults{}, // backs read_tool_result
		AgentMemorySearch:  allWiredAgentMemorySearch{},
		ConversationSearch: allWiredConversationSearch{},
		SkillProposals:     allWiredSkillProposals{},
		Online: OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"example.com"}, // backs http_request
		},
		Interrupt: func(context.Context, string, runs.Interrupt) (interrupt.Resolution, error) {
			return interrupt.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	wireCreateGoal(t, built.Resolver)

	manifest, err := built.Resolver.Manifest(t.Context(), tool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	resolved := manifestTools(manifest)
	existing := definitionNames(resolved)
	checkedContracts := make(map[string]bool)
	for _, candidate := range resolved {
		name := candidate.Definition().Name
		if checkedContracts[name] {
			continue
		}
		checkedContracts[name] = true
		assertBuiltInToolContract(t, candidate)
	}
	// Agent Framework advertises delegate_task from the Interaction Definition rather than
	// the ordinary Tool manifest.
	existing[toolname.DelegateTask] = true

	declared := make(map[string]bool)
	var unreachable []string
	for name, descriptor := range descriptors() {
		if declared[name] {
			t.Errorf("built-in identity %q is declared more than once", name)
		}
		declared[name] = true
		if !descriptor.safety.Valid() {
			t.Errorf("built-in identity %q has invalid safety class %q", name, descriptor.safety)
		}
		if (descriptor.activityText == "") == (descriptor.activity == nil) {
			t.Errorf("built-in identity %q must define exactly one static or argument-aware activity", name)
		}
		if descriptor.activityText != "" && !isConciseActivityText(descriptor.activityText, 120) {
			t.Errorf("built-in identity %q has invalid static activity %q", name, descriptor.activityText)
		}
		if (descriptor.result.project == nil) != (descriptor.result.resultType == nil) {
			t.Errorf("built-in identity %q must pair its result projection with its published type", name)
		}
		if !existing[name] {
			unreachable = append(unreachable, name)
		}
	}
	if len(unreachable) > 0 {
		slices.Sort(unreachable)
		t.Errorf("the descriptor catalog classifies %v, which no built tool answers to — "+
			"either the tool is gone (drop the name) or it was never built (wire it)", unreachable)
	}
	var unclassified []string
	for name := range existing {
		if !declared[name] {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		slices.Sort(unclassified)
		t.Errorf("built-in tools %v rely on unknown-tool fallbacks — describe each one explicitly", unclassified)
	}
}
