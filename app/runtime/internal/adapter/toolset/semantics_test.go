package toolset

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestSemanticsSafetyClassFailsClosed(t *testing.T) {
	semantics := Semantics{}
	for _, test := range []struct {
		name string
		want tool.SafetyClass
	}{
		{name: toolNameDelegateTask, want: tool.SafetyClassSafe},
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
