package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestSemanticsSafetyClassFailsClosed(t *testing.T) {
	interpreter := Interpreter{}
	for _, test := range []struct {
		name string
		want tool.SafetyClass
	}{
		{name: catalog.DelegateTask, want: tool.SafetyClassSafe},
		{name: catalog.ListSchedules, want: tool.SafetyClassSafe},
		{name: catalog.CreateSchedule, want: tool.SafetyClassWrite},
		{name: catalog.ApplyPatch, want: tool.SafetyClassWrite},
		{name: catalog.Shell, want: tool.SafetyClassExec},
		{name: catalog.ReadShellOutput, want: tool.SafetyClassSafe},
		{name: catalog.StopShell, want: tool.SafetyClassExec},
		{name: catalog.WebFetch, want: tool.SafetyClassNetwork},
		{name: catalog.HTTPRequest, want: tool.SafetyClassNetwork},
		{name: "unknown_tool", want: tool.SafetyClassExec},
	} {
		if got := interpreter.SafetyClass(test.name); got != test.want {
			t.Errorf("SafetyClass(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestSemanticsApprovalSubject(t *testing.T) {
	interpreter := Interpreter{}
	for _, test := range []struct {
		name      string
		arguments string
		want      string
		wantError bool
	}{
		{name: catalog.Shell, arguments: `{"command":"npm run build"}`, want: "npm run build"},
		{name: catalog.Read, arguments: `{"path":"go.mod"}`, want: "go.mod"},
		{name: catalog.ApplyPatch, arguments: `{"patch":"diff"}`},
		{name: catalog.Grep, arguments: `{"pattern":"foo"}`},
		{name: catalog.StopShell, arguments: `{"shell_id":"s1"}`},
		{name: catalog.Shell, arguments: `{"timeout_millis":5}`, wantError: true},
	} {
		arguments, err := tool.ParseArguments(test.arguments)
		if err != nil {
			t.Fatalf("ParseArguments(%q): %v", test.arguments, err)
		}
		got, err := interpreter.ApprovalSubject(test.name, arguments)
		if test.wantError {
			if !errors.Is(err, tool.ErrInvalidArguments) {
				t.Errorf("ApprovalSubject(%q) error = %v, want invalid arguments", test.name, err)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("ApprovalSubject(%q) = (%q, %v), want %q", test.name, got, err, test.want)
		}
	}
}

func TestSemanticsShellCommand(t *testing.T) {
	interpreter := Interpreter{}
	if got := interpreter.ShellCommand(catalog.Shell, `{"command":"rm -rf /"}`); got != "rm -rf /" {
		t.Fatalf("ShellCommand = %q, want command", got)
	}
	if got := interpreter.ShellCommand(catalog.ApplyPatch, `{"command":"rm -rf /"}`); got != "" {
		t.Fatalf("non-shell command = %q, want empty", got)
	}
}

func TestSemanticsDelegationUsesChildLifecyclePolicy(t *testing.T) {
	interpreter := Interpreter{}
	if interpreter.UsesStandardPolicy(catalog.DelegateTask) {
		t.Fatal("delegation entered ordinary tool-call policy")
	}
	for _, name := range []string{catalog.Read, "extension_tool"} {
		if !interpreter.UsesStandardPolicy(name) {
			t.Errorf("ordinary tool %q bypassed standard policy", name)
		}
	}
}

func TestSemanticsProjectsSuccessfulPlanReplacement(t *testing.T) {
	want := plan.State{Revision: 7, Steps: []plan.Step{{Description: "verify", Status: plan.StatusInProgress}}}
	interpreter := NewInterpreter(fixedPlanState{state: want})
	event, err := interpreter.ProjectOutcome(t.Context(), "session_1", catalog.SetPlan, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := event.(runs.PlanUpdated)
	if !ok || updated.State.Revision != want.Revision || len(updated.State.Steps) != 1 {
		t.Fatalf("projected event = %#v, want PlanUpdated %#v", event, want)
	}
	for _, test := range []struct {
		name      string
		succeeded bool
	}{
		{name: catalog.SetPlan},
		{name: catalog.Read, succeeded: true},
	} {
		event, err := interpreter.ProjectOutcome(t.Context(), "session_1", test.name, test.succeeded)
		if err != nil || event != nil {
			t.Errorf("ProjectOutcome(%q, %t) = (%#v, %v), want no event", test.name, test.succeeded, event, err)
		}
	}
}

type fixedPlanState struct {
	state plan.State
	err   error
}

func (s fixedPlanState) State(context.Context, string) (plan.State, error) {
	return s.state, s.err
}
