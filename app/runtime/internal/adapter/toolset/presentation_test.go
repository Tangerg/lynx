package toolset

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestPresenterActivity(t *testing.T) {
	presenter := Presenter{}
	if got := presenter.Activity(toolNameWebSearch); got != "Searching the web" {
		t.Fatalf("web search activity = %q", got)
	}
	if got := presenter.Activity(toolNameProposeSkill); got != "Proposing a Skill" {
		t.Fatalf("propose Skill activity = %q", got)
	}
	if got := presenter.Activity("external_tool"); got != "" {
		t.Fatalf("unknown activity = %q, want empty", got)
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
