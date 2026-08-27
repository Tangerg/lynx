package toolset

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
)

func TestPresenterActivity(t *testing.T) {
	presenter := Presenter{}
	tests := []struct {
		name      string
		toolName  string
		arguments string
		want      string
	}{
		{name: "web search", toolName: tool.WebSearch, arguments: `{}`, want: "Searching the web"},
		{name: "shell description", toolName: tool.Shell, arguments: `{"command":"go test ./...","description":"Run server tests"}`, want: "Run server tests"},
		{name: "shell invalid description", toolName: tool.Shell, arguments: `{"description":" Run server tests"}`, want: "Running command"},
		{name: "delegation summary", toolName: tool.DelegateTask, arguments: `{"summary":"Review tool contracts"}`, want: "Delegating: Review tool contracts"},
		{name: "long delegation summary", toolName: tool.DelegateTask, arguments: `{"summary":"` + strings.Repeat("a", 81) + `"}`, want: "Delegating to a sub-agent"},
		{name: "enter Plan mode", toolName: tool.EnterPlanMode, arguments: `{}`, want: "Entering Plan mode"},
		{name: "set Plan", toolName: tool.SetPlan, arguments: `{}`, want: "Updating the Plan"},
		{name: "exit Plan mode", toolName: tool.ExitPlanMode, arguments: `{}`, want: "Requesting Plan approval"},
		{name: "create Goal", toolName: tool.CreateGoal, arguments: `{"objective":"finish the work"}`, want: "Starting an autonomous Goal"},
		{name: "create titled schedule", toolName: tool.CreateSchedule, arguments: `{"title":"Daily review"}`, want: "Creating schedule: Daily review"},
		{name: "create untitled schedule", toolName: tool.CreateSchedule, arguments: `{}`, want: "Creating a schedule"},
		{name: "load Skill", toolName: tool.LoadSkill, arguments: `{"name":"go-review"}`, want: "Loading Skill: go-review"},
		{name: "propose named Skill", toolName: tool.ProposeSkill, arguments: `{"name":"review-go-api"}`, want: "Proposing Skill: review-go-api"},
		{name: "propose unnamed Skill", toolName: tool.ProposeSkill, arguments: `{}`, want: "Proposing a Skill"},
		{name: "LSP references", toolName: tool.LSP, arguments: `{"operation":"references"}`, want: "Finding symbol references"},
		{name: "unknown LSP operation", toolName: tool.LSP, arguments: `{"operation":"rename"}`, want: "Querying the language server"},
		{name: "HTTP default method", toolName: tool.HTTPRequest, arguments: `{"url":"https://example.com"}`, want: "Sending GET request"},
		{name: "HTTP explicit method", toolName: tool.HTTPRequest, arguments: `{"url":"https://example.com","method":"POST"}`, want: "Sending POST request"},
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
		tool.Shell,
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
		tool.ApplyPatch,
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

func TestPresenterSearchResult(t *testing.T) {
	presented, _ := Presenter{}.Present(
		tool.Grep,
		tool.Arguments{},
		mustToolResult(t, map[string]any{"matches": []any{
			map[string]any{"path": "main.go", "line": 7, "text": "func main()"},
		}}),
	)
	want := map[string]any{"hits": []any{
		map[string]any{"path": "main.go", "lineNumber": json.Number("7"), "snippet": "func main()"},
	}}
	if got := presented.Any(); !reflect.DeepEqual(got, want) {
		t.Fatalf("presented search = %#v, want %#v", got, want)
	}
}

func TestPresenterWebSearchResult(t *testing.T) {
	presented, _ := Presenter{}.Present(
		tool.WebSearch,
		tool.Arguments{},
		mustToolResult(t, map[string]any{"results": []any{
			map[string]any{"title": "Example", "url": "https://example.com", "favicon_url": "https://example.com/icon.png"},
		}}),
	)
	want := map[string]any{"results": []any{
		map[string]any{"title": "Example", "url": "https://example.com", "faviconUrl": "https://example.com/icon.png"},
	}}
	if got := presented.Any(); !reflect.DeepEqual(got, want) {
		t.Fatalf("presented web search = %#v, want %#v", got, want)
	}
}

func TestPublishedResultContractsDecodePresenterOutput(t *testing.T) {
	contracts := make(map[string]PresentationContract)
	for _, contract := range PresentationContracts() {
		if _, exists := contracts[contract.ToolName]; exists {
			t.Fatalf("duplicate result contract for %q", contract.ToolName)
		}
		contracts[contract.ToolName] = contract
	}

	tests := []struct {
		name   string
		result map[string]any
	}{
		{name: tool.Shell, result: map[string]any{"stdout": "ok", "exit_code": 0}},
		{name: tool.Glob, result: map[string]any{"paths": []string{"main.go"}}},
		{name: tool.Grep, result: map[string]any{"matches": []any{map[string]any{"path": "main.go", "line": 1, "text": "package main"}}}},
		{name: tool.WebSearch, result: map[string]any{"results": []any{map[string]any{"url": "https://example.com"}}}},
		{name: tool.ApplyPatch, result: map[string]any{"files": []any{map[string]any{"path": "main.go"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, ok := contracts[test.name]
			if !ok {
				t.Fatalf("no published result contract for %q", test.name)
			}
			presented, _ := Presenter{}.Present(test.name, tool.Arguments{}, mustToolResult(t, test.result))
			encoded, err := json.Marshal(presented)
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(reflect.New(contract.ResultType).Interface()); err != nil {
				t.Fatalf("presented result does not match %v: %v", contract.ResultType, err)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				t.Fatalf("presented result has trailing JSON: %v", err)
			}
		})
	}

	wantStatuses := []string{"added", "deleted", "modified", "moved"}
	if got := contracts[tool.ApplyPatch].EnumValues[reflect.TypeFor[ChangeStatus]()]; !reflect.DeepEqual(got, wantStatuses) {
		t.Fatalf("published patch statuses = %v, want %v", got, wantStatuses)
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
