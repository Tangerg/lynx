package toolset

import (
	"context"
	"errors"
	"testing"
	"time"

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
		{name: tool.DelegateTask, want: tool.SafetyClassSafe},
		{name: tool.ListSchedules, want: tool.SafetyClassSafe},
		{name: tool.CreateSchedule, want: tool.SafetyClassWrite},
		{name: tool.ApplyPatch, want: tool.SafetyClassWrite},
		{name: tool.Shell, want: tool.SafetyClassExec},
		{name: tool.ReadShellOutput, want: tool.SafetyClassSafe},
		{name: tool.StopShell, want: tool.SafetyClassExec},
		{name: tool.WebFetch, want: tool.SafetyClassNetwork},
		{name: tool.HTTPRequest, want: tool.SafetyClassNetwork},
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
		{name: tool.Shell, arguments: `{"command":"npm run build"}`, want: "npm run build"},
		{name: tool.Read, arguments: `{"path":"go.mod"}`, want: "go.mod"},
		{name: tool.ApplyPatch, arguments: `{"patch":"diff"}`},
		{name: tool.Grep, arguments: `{"pattern":"foo"}`},
		{name: tool.StopShell, arguments: `{"shell_id":"s1"}`},
		{name: tool.Shell, arguments: `{"timeout_millis":5}`, wantError: true},
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
	if got := interpreter.ShellCommand(tool.Shell, `{"command":"rm -rf /"}`); got != "rm -rf /" {
		t.Fatalf("ShellCommand = %q, want command", got)
	}
	if got := interpreter.ShellCommand(tool.ApplyPatch, `{"command":"rm -rf /"}`); got != "" {
		t.Fatalf("non-shell command = %q, want empty", got)
	}
}

func TestSemanticsDelegationUsesChildLifecyclePolicy(t *testing.T) {
	interpreter := Interpreter{}
	if interpreter.UsesStandardPolicy(tool.DelegateTask) {
		t.Fatal("delegation entered ordinary tool-call policy")
	}
	for _, name := range []string{tool.Read, "extension_tool"} {
		if !interpreter.UsesStandardPolicy(name) {
			t.Errorf("ordinary tool %q bypassed standard policy", name)
		}
	}
}

func TestSemanticsProjectsSuccessfulPlanReplacement(t *testing.T) {
	want, err := plan.Restore(plan.Snapshot{
		Revision: 7, UpdatedAt: time.Now(),
		Steps: []plan.Step{{Description: "verify", Status: plan.StatusInProgress}},
	})
	if err != nil {
		t.Fatal(err)
	}
	interpreter := NewInterpreter(fixedPlanState{state: want})
	event, err := interpreter.ProjectOutcome(t.Context(), "session_1", tool.SetPlan, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := event.(runs.PlanUpdated)
	if !ok || updated.State.Revision() != want.Revision() || len(updated.State.Steps()) != 1 {
		t.Fatalf("projected event = %#v, want PlanUpdated %#v", event, want)
	}
	for _, test := range []struct {
		name      string
		succeeded bool
	}{
		{name: tool.SetPlan},
		{name: tool.Read, succeeded: true},
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

func (f fixedPlanState) State(context.Context, string) (plan.State, error) {
	return f.state, f.err
}
