package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestSemanticsSafetyClassFailsClosed(t *testing.T) {
	semantics := Semantics{}
	for _, test := range []struct {
		name string
		want tool.SafetyClass
	}{
		{name: delegation.Name, want: tool.SafetyClassSafe},
		{name: toolNameListSchedules, want: tool.SafetyClassSafe},
		{name: toolNameCreateSchedule, want: tool.SafetyClassWrite},
		{name: toolNameWrite, want: tool.SafetyClassWrite},
		{name: toolNameShell, want: tool.SafetyClassExec},
		{name: toolNameReadShellOutput, want: tool.SafetyClassSafe},
		{name: toolNameStopShell, want: tool.SafetyClassExec},
		{name: toolNameWebFetch, want: tool.SafetyClassNetwork},
		{name: toolNameHTTPRequest, want: tool.SafetyClassNetwork},
		{name: "unknown_tool", want: tool.SafetyClassExec},
	} {
		if got := semantics.SafetyClass(test.name); got != test.want {
			t.Errorf("SafetyClass(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestSemanticsApprovalSubject(t *testing.T) {
	semantics := Semantics{}
	for _, test := range []struct {
		name      string
		arguments string
		want      string
		wantError bool
	}{
		{name: toolNameShell, arguments: `{"command":"npm run build"}`, want: "npm run build"},
		{name: toolNameEdit, arguments: `{"path":"src/a.go"}`, want: "src/a.go"},
		{name: toolNameRead, arguments: `{"path":"go.mod"}`, want: "go.mod"},
		{name: toolNameWrite, arguments: `{"path":"out.txt"}`, want: "out.txt"},
		{name: toolNameGrep, arguments: `{"pattern":"foo"}`},
		{name: toolNameStopShell, arguments: `{"shell_id":"s1"}`},
		{name: toolNameShell, arguments: `{"timeout_ms":5}`, wantError: true},
	} {
		arguments, err := tool.ParseArguments(test.arguments)
		if err != nil {
			t.Fatalf("ParseArguments(%q): %v", test.arguments, err)
		}
		got, err := semantics.ApprovalSubject(test.name, arguments)
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
	semantics := Semantics{}
	if got := semantics.ShellCommand(toolNameShell, `{"command":"rm -rf /"}`); got != "rm -rf /" {
		t.Fatalf("ShellCommand = %q, want command", got)
	}
	if got := semantics.ShellCommand(toolNameWrite, `{"command":"rm -rf /"}`); got != "" {
		t.Fatalf("non-shell command = %q, want empty", got)
	}
}

func TestSemanticsDelegationUsesChildLifecyclePolicy(t *testing.T) {
	semantics := Semantics{}
	if semantics.UsesStandardPolicy(delegation.Name) {
		t.Fatal("delegation entered ordinary tool-call policy")
	}
	for _, name := range []string{toolNameRead, "extension_tool"} {
		if !semantics.UsesStandardPolicy(name) {
			t.Errorf("ordinary tool %q bypassed standard policy", name)
		}
	}
}

func TestSemanticsProjectsSuccessfulPlanReplacement(t *testing.T) {
	want := plan.State{Revision: 7, Steps: []plan.Step{{Description: "verify", Status: plan.StatusInProgress}}}
	semantics := NewSemantics(fixedPlanState{state: want})
	event, err := semantics.ProjectOutcome(t.Context(), "session_1", toolNameSetPlan, true)
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
		{name: toolNameSetPlan},
		{name: toolNameRead, succeeded: true},
	} {
		event, err := semantics.ProjectOutcome(t.Context(), "session_1", test.name, test.succeeded)
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
