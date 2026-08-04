package toolset

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestPresenterActivity(t *testing.T) {
	presenter := Presenter{}
	tests := []struct {
		name      string
		toolName  string
		arguments string
		want      string
	}{
		{name: "web search", toolName: catalog.WebSearch, arguments: `{}`, want: "Searching the web"},
		{name: "shell description", toolName: catalog.Shell, arguments: `{"command":"go test ./...","description":"Run server tests"}`, want: "Run server tests"},
		{name: "shell invalid description", toolName: catalog.Shell, arguments: `{"description":" Run server tests"}`, want: "Running command"},
		{name: "delegation summary", toolName: catalog.DelegateTask, arguments: `{"summary":"Review tool contracts"}`, want: "Delegating: Review tool contracts"},
		{name: "long delegation summary", toolName: catalog.DelegateTask, arguments: `{"summary":"` + strings.Repeat("a", 81) + `"}`, want: "Delegating to a sub-agent"},
		{name: "enter Plan mode", toolName: catalog.EnterPlanMode, arguments: `{}`, want: "Entering Plan mode"},
		{name: "set Plan", toolName: catalog.SetPlan, arguments: `{}`, want: "Updating the Plan"},
		{name: "exit Plan mode", toolName: catalog.ExitPlanMode, arguments: `{}`, want: "Requesting Plan approval"},
		{name: "create Goal", toolName: catalog.CreateGoal, arguments: `{"objective":"finish the work"}`, want: "Starting an autonomous Goal"},
		{name: "create titled schedule", toolName: catalog.CreateSchedule, arguments: `{"title":"Daily review"}`, want: "Creating schedule: Daily review"},
		{name: "create untitled schedule", toolName: catalog.CreateSchedule, arguments: `{}`, want: "Creating a schedule"},
		{name: "load Skill", toolName: catalog.LoadSkill, arguments: `{"name":"go-review"}`, want: "Loading Skill: go-review"},
		{name: "propose named Skill", toolName: catalog.ProposeSkill, arguments: `{"name":"review-go-api"}`, want: "Proposing Skill: review-go-api"},
		{name: "propose unnamed Skill", toolName: catalog.ProposeSkill, arguments: `{}`, want: "Proposing a Skill"},
		{name: "LSP references", toolName: catalog.LSP, arguments: `{"operation":"references"}`, want: "Finding symbol references"},
		{name: "unknown LSP operation", toolName: catalog.LSP, arguments: `{"operation":"rename"}`, want: "Querying the language server"},
		{name: "HTTP default method", toolName: catalog.HTTPRequest, arguments: `{"url":"https://example.com"}`, want: "Sending GET request"},
		{name: "HTTP explicit method", toolName: catalog.HTTPRequest, arguments: `{"url":"https://example.com","method":"POST"}`, want: "Sending POST request"},
		{name: "unknown tool", toolName: "external_tool", arguments: `{}`, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := tool.ParseArguments(test.arguments)
			if err != nil {
				t.Fatal(err)
			}
			if got := presenter.Activity(test.toolName, arguments); got != test.want {
				t.Fatalf("activity = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPresenterCommandResult(t *testing.T) {
	presented, outputText := Presenter{}.Present(
		catalog.Shell,
		tool.Arguments{},
		mustToolResult(t, map[string]any{"stdout": "out", "stderr": "err", "exit_code": 0}),
	)
	want := map[string]any{"output": "out\nerr", "exitCode": json.Number("0")}
	if got := presented.Any(); !reflect.DeepEqual(got, want) {
		t.Fatalf("presented command = %#v, want %#v", got, want)
	}
	if outputText != "out\nerr" {
		t.Fatalf("output text = %q", outputText)
	}
}

func TestPresenterApplyPatchResult(t *testing.T) {
	presented, _ := Presenter{}.Present(
		catalog.ApplyPatch,
		tool.Arguments{},
		mustToolResult(t, map[string]any{"files": []any{
			map[string]any{"path": "new.go", "created": true},
			map[string]any{"path": "next.go", "moved_from": "old.go"},
		}}),
	)
	want := map[string]any{"changes": []any{
		map[string]any{"path": "new.go", "status": "added"},
		map[string]any{"path": "next.go", "status": "moved", "from": "old.go"},
	}}
	if got := presented.Any(); !reflect.DeepEqual(got, want) {
		t.Fatalf("presented patch = %#v, want %#v", got, want)
	}
}

func TestPresenterKeepsUnknownToolResult(t *testing.T) {
	original := mustToolResult(t, map[string]any{"custom": true})
	presented, outputText := Presenter{}.Present("external_tool", tool.Arguments{}, original)
	if !reflect.DeepEqual(presented.Any(), original.Any()) || outputText != "" {
		t.Fatalf("unknown presentation = %#v, %q", presented.Any(), outputText)
	}
}

func mustToolResult(t *testing.T, value any) tool.Result {
	t.Helper()
	result, err := tool.NewResult(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
