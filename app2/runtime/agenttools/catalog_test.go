package agenttools_test

import (
	"context"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	lyraskills "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/agenttools"
	"github.com/Tangerg/lynx/app2/runtime/codeintel"
	"github.com/Tangerg/lynx/app2/runtime/domain/approvalpolicy"
	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/shellflow"
)

func TestBuiltInToolInventoryMatchesPublishedThirtyToolSurface(t *testing.T) {
	workspace := t.TempDir()
	shells, err := shellflow.New(t.Context())
	if err != nil {
		t.Fatalf("shellflow.New() error = %v", err)
	}
	t.Cleanup(shells.Close)
	code, err := codeintel.New(t.Context(), []codeintel.ServerSpec{{
		Name: "test", Command: "missing-language-server", LanguageID: "go",
		Extensions: []string{".go"},
	}})
	if err != nil {
		t.Fatalf("codeintel.New() error = %v", err)
	}
	t.Cleanup(func() { _ = code.Close() })

	gateways := testGateways{skills: lyraskills.NewFS(fstest.MapFS{})}
	catalog, err := agenttools.New(agenttools.Config{
		Policy: gateways, Results: gateways, Goals: gateways, Plans: planGateways{gateways},
		Schedules: gateways, Skills: gateways, Memory: gateways,
		Conversations: gateways, Hooks: gateways, Shells: shells, CodeIntel: code,
		Online: agenttools.OnlineConfig{
			JinaAPIKey: "test", TavilyAPIKey: "test",
			HTTPAllowedHosts: []string{"example.test"}, HTTPAllowedMethods: []string{"GET"},
		},
	})
	if err != nil {
		t.Fatalf("agenttools.New() error = %v", err)
	}
	tools, err := catalog.ForRun(t.Context(), agentexec.ToolScope{
		SessionID: "ses_test", RunID: "run_test", Workspace: workspace,
		IsRootRun: true, Facts: testFacts{},
	})
	if err != nil {
		t.Fatalf("Catalog.ForRun() error = %v", err)
	}

	names := make([]string, 0, len(tools)+1)
	for _, executable := range tools {
		names = append(names, executable.Tool.Definition().Name)
	}
	// Delegation is supplied by agentexec's deployment family rather than the
	// ordinary executable catalog, but is part of the same model-visible set.
	names = append(names, agentexec.DelegateToolName)
	slices.Sort(names)

	expected := []string{
		"apply_patch", "ask_user", "create_goal", "create_schedule", "delegate_task",
		"delete_schedule", "enter_plan_mode", "exit_plan_mode", "get_goal", "glob",
		"grep", "http_request", "list_schedules", "list_skills", "load_skill", "lsp",
		"propose_skill", "read", "read_shell_output", "read_skill_resource",
		"read_tool_result", "report_goal_outcome", "search_conversations", "search_memory",
		"search_tools", "set_plan", "shell", "stop_shell", "web_fetch", "web_search",
	}
	if !slices.Equal(names, expected) {
		t.Fatalf("built-in tools = %v, want %v", names, expected)
	}
}

type testGateways struct{ skills lyraskills.ResourceSource }

func (testGateways) Mode(context.Context) (approvalpolicy.Mode, error) {
	return approvalpolicy.ModeBalanced, nil
}
func (testGateways) Decide(context.Context, approvalpolicy.Query) (approvalpolicy.Decision, bool, error) {
	return "", false, nil
}
func (testGateways) Remember(context.Context, approvalpolicy.Remember) error { return nil }
func (testGateways) ReadToolResult(context.Context, string, string) (toolresult.Record, error) {
	return toolresult.Record{}, fs.ErrNotExist
}
func (testGateways) Start(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error) {
	return &protocol.Goal{}, nil
}
func (testGateways) Get(context.Context, protocol.GoalRequest) (*protocol.Goal, error) {
	return &protocol.Goal{}, nil
}
func (testGateways) IsOwnedRun(context.Context, string, string) (bool, error) { return true, nil }
func (testGateways) Report(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (testGateways) Replace(context.Context, string, []protocol.PlanStep) (*protocol.Plan, error) {
	return &protocol.Plan{}, nil
}
func (testGateways) EnterMode(context.Context, string) (bool, error) { return true, nil }
func (testGateways) ExitMode(context.Context, string) (bool, error)  { return true, nil }
func (testGateways) List(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
	return &protocol.Page[protocol.Schedule]{}, nil
}
func (testGateways) Create(context.Context, protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	return &protocol.Schedule{}, nil
}
func (testGateways) Delete(context.Context, protocol.DeleteScheduleRequest) error { return nil }
func (gateways testGateways) Source(context.Context, string) (lyraskills.ResourceSource, error) {
	return gateways.skills, nil
}
func (testGateways) ProposeSkill(context.Context, agenttools.SkillProposalDraft) (*protocol.SkillProposal, error) {
	return &protocol.SkillProposal{}, nil
}
func (testGateways) SearchMemory(context.Context, string, string, int) ([]agenttools.MemoryHit, error) {
	return nil, nil
}
func (testGateways) SearchConversations(context.Context, string, string, string, int) ([]transcript.SearchHit, error) {
	return nil, nil
}
func (testGateways) Evaluate(context.Context, lifecyclehook.Invocation) (lifecyclehook.Decision, error) {
	return lifecyclehook.Decision{}, nil
}
func (testGateways) EvaluateBestEffort(context.Context, lifecyclehook.Invocation) lifecyclehook.Decision {
	return lifecyclehook.Decision{}
}

// Go cannot overload Mode for the approval and Plan ports, so a small wrapper
// supplies the Plan method while sharing all other gateway behavior.
type planGateways struct{ testGateways }

func (planGateways) Get(context.Context, protocol.GetPlanRequest) (*protocol.Plan, error) {
	return &protocol.Plan{}, nil
}
func (planGateways) Mode(context.Context, string) (bool, error) { return false, nil }

type testFacts struct{}

func (testFacts) RecordCommittedPlan(string, protocol.Plan)           {}
func (testFacts) RecordEffectiveToolArguments(string, map[string]any) {}
