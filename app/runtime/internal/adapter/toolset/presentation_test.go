package toolset

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
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
		{name: "web search", toolName: toolNameWebSearch, arguments: `{}`, want: "Searching the web"},
		{name: "shell description", toolName: toolNameShell, arguments: `{"command":"go test ./...","description":"Run server tests"}`, want: "Run server tests"},
		{name: "shell invalid description", toolName: toolNameShell, arguments: `{"description":" Run server tests"}`, want: "Running command"},
		{name: "delegation summary", toolName: delegation.Name, arguments: `{"summary":"Review tool contracts"}`, want: "Delegating: Review tool contracts"},
		{name: "long delegation summary", toolName: delegation.Name, arguments: `{"summary":"` + strings.Repeat("a", 81) + `"}`, want: "Delegating to a sub-agent"},
		{name: "enter Plan mode", toolName: toolNameEnterPlanMode, arguments: `{}`, want: "Entering Plan mode"},
		{name: "set Plan", toolName: toolNameSetPlan, arguments: `{}`, want: "Updating the Plan"},
		{name: "exit Plan mode", toolName: toolNameExitPlanMode, arguments: `{}`, want: "Requesting Plan approval"},
		{name: "create Goal", toolName: toolNameCreateGoal, arguments: `{"objective":"finish the work"}`, want: "Starting an autonomous Goal"},
		{name: "create titled schedule", toolName: toolNameCreateSchedule, arguments: `{"title":"Daily review"}`, want: "Creating schedule: Daily review"},
		{name: "create untitled schedule", toolName: toolNameCreateSchedule, arguments: `{}`, want: "Creating a schedule"},
		{name: "load Skill", toolName: toolNameLoadSkill, arguments: `{"name":"go-review"}`, want: "Loading Skill: go-review"},
		{name: "propose named Skill", toolName: toolNameProposeSkill, arguments: `{"name":"review-go-api"}`, want: "Proposing Skill: review-go-api"},
		{name: "propose unnamed Skill", toolName: toolNameProposeSkill, arguments: `{}`, want: "Proposing a Skill"},
		{name: "LSP references", toolName: toolNameLSP, arguments: `{"operation":"references"}`, want: "Finding symbol references"},
		{name: "unknown LSP operation", toolName: toolNameLSP, arguments: `{"operation":"rename"}`, want: "Querying the language server"},
		{name: "HTTP default method", toolName: toolNameHTTPRequest, arguments: `{"url":"https://example.com"}`, want: "Sending GET request"},
		{name: "HTTP explicit method", toolName: toolNameHTTPRequest, arguments: `{"url":"https://example.com","method":"POST"}`, want: "Sending POST request"},
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
		toolNameShell,
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

func TestPresenterEditResult(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"a.go","old_string":"old\n","new_string":"new\n"}`)
	if err != nil {
		t.Fatal(err)
	}
	presented, _ := Presenter{}.Present(
		toolNameEdit,
		arguments,
		mustToolResult(t, map[string]any{"replacements": 1}),
	)
	want := map[string]any{"changes": []any{map[string]any{
		"path": "a.go", "status": "modified", "diff": []any{
			map[string]any{"type": "deleted", "leftLine": json.Number("1"), "code": "old"},
			map[string]any{"type": "added", "rightLine": json.Number("1"), "code": "new"},
		},
	}}}
	if got := presented.Any(); !reflect.DeepEqual(got, want) {
		t.Fatalf("presented edit = %#v, want %#v", got, want)
	}
}

func TestPresenterApplyPatchResult(t *testing.T) {
	presented, _ := Presenter{}.Present(
		toolNameApplyPatch,
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
