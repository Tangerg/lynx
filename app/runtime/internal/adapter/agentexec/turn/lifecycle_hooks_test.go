package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestTurnLifecycle_SubagentHooks(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.SubagentStart, Command: "record", Source: "test"},
		{Event: hooks.SubagentStop, Command: "record", Source: "test"},
	}, hooks.NewRunner(rec, nil))
	lifecycle := &turnLifecycle{
		rootID:    "root",
		sessionID: "sess",
		cwd:       "/work",
		hooks:     bound,
	}
	listener := lifecycle.listener("turn")

	listener.OnEvent(context.Background(), event.ProcessCreated{Header: event.NewHeader("root")})
	listener.OnEvent(context.Background(), event.ProcessCreated{
		Header: event.NewHeader("child"),
		Bindings: map[string]any{core.DefaultBindingName: testTaskInput{
			Description: "inspect auth",
			Prompt:      "Find where auth failures are handled.",
		}},
	})
	listener.OnEvent(context.Background(), event.ProcessCompleted{
		Header: event.NewHeader("child"),
		Result: "auth failures are handled in middleware",
	})

	if len(rec.inputs) != 2 {
		t.Fatalf("hook inputs = %d, want 2: %#v", len(rec.inputs), rec.inputs)
	}
	start := rec.inputs[0].Subagent
	if rec.inputs[0].Event != hooks.SubagentStart || start.ProcessID != "child" || start.ParentProcessID != "root" {
		t.Fatalf("start input = %+v", rec.inputs[0])
	}
	if start.Description != "inspect auth" || start.Prompt != "Find where auth failures are handled." {
		t.Fatalf("start subagent = %+v", start)
	}
	stop := rec.inputs[1].Subagent
	if rec.inputs[1].Event != hooks.SubagentStop || rec.inputs[1].Reason != "process_completed" {
		t.Fatalf("stop input = %+v", rec.inputs[1])
	}
	if stop.Status != "completed" || stop.Result != "auth failures are handled in middleware" || stop.Description != "inspect auth" {
		t.Fatalf("stop subagent = %+v", stop)
	}
}

func TestTurnLifecycleBindsRootBeforeChildTerminal(t *testing.T) {
	lifecycle := &turnLifecycle{}
	listener := lifecycle.listener("turn")

	listener.OnEvent(t.Context(), event.ProcessCreated{Header: event.NewHeader("root")})
	listener.OnEvent(t.Context(), event.ProcessCompleted{Header: event.NewHeader("child")})
	listener.OnEvent(t.Context(), event.ProcessKilled{Header: event.NewHeader("root")})

	terminal := lifecycle.terminalEvent()
	if terminal == nil || terminal.ProcessID() != "root" {
		t.Fatalf("terminal = %#v, want root terminal", terminal)
	}
	if _, ok := terminal.(event.ProcessKilled); !ok {
		t.Fatalf("terminal type = %T, want event.ProcessKilled", terminal)
	}
}

type testTaskInput struct {
	Description string
	Prompt      string
}

func (in testTaskInput) SubagentDescription() string { return in.Description }

func (in testTaskInput) SubagentPrompt() string { return in.Prompt }

func TestSubagentTaskInputRequiresTypedDefaultBinding(t *testing.T) {
	task := testTaskInput{Description: "inspect auth", Prompt: "Find where auth failures are handled."}
	for _, test := range []struct {
		name        string
		bindings    map[string]any
		description string
		prompt      string
	}{
		{name: "typed default", bindings: map[string]any{core.DefaultBindingName: task}, description: task.Description, prompt: task.Prompt},
		{name: "dynamic map", bindings: map[string]any{core.DefaultBindingName: map[string]any{"description": task.Description, "prompt": task.Prompt}}},
		{name: "non-default binding", bindings: map[string]any{"task": task}},
		{name: "nil bindings"},
	} {
		t.Run(test.name, func(t *testing.T) {
			description, prompt := subagentTaskInput(test.bindings)
			if description != test.description || prompt != test.prompt {
				t.Fatalf("subagentTaskInput = %q, %q; want %q, %q", description, prompt, test.description, test.prompt)
			}
		})
	}
}

func TestSummarizeHookText_KeepsUTF8Boundary(t *testing.T) {
	got := summarizeHookText(strings.Repeat("界", 1000))
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("summary suffix = %q", got[len(got)-20:])
	}
	if !utf8.ValidString(got) {
		t.Fatalf("summary is not valid UTF-8: %q", got)
	}
}

type recordHookCommands struct {
	inputs []hooks.Input
}

func (r *recordHookCommands) RunHookCommand(_ context.Context, req hooks.CommandRequest) hooks.CommandResult {
	var in hooks.Input
	_ = json.Unmarshal(req.Stdin, &in)
	r.inputs = append(r.inputs, in)
	return hooks.CommandResult{}
}
