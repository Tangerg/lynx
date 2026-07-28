package turn

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestSubagentLifecycleHooks(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.SubagentStart, Command: "record", Source: "test"},
		{Event: hooks.SubagentStop, Command: "record", Source: "test"},
	}, hooks.NewRunner(rec, nil))
	lifecycle := &subagentLifecycle{
		rootID:    "root",
		sessionID: "sess",
		cwd:       "/work",
		hooks:     bound,
		project: func(processID string) (agentexec.SubagentProjection, bool) {
			if processID == "child" {
				return agentexec.SubagentProjection{
					ParentProcessID: "root",
					Description:     "inspect auth",
					Prompt:          "Find where auth failures are handled.",
					Reply:           "auth failures are handled in middleware",
				}, true
			}
			return agentexec.SubagentProjection{}, false
		},
	}
	listener := lifecycle.listener("turn")

	listener.OnEvent(context.Background(), event.ProcessCreated{Header: event.NewHeader("root")})
	listener.OnEvent(context.Background(), event.ProcessCreated{
		Header:   event.NewHeader("child"),
		ParentID: "root",
	})
	listener.OnEvent(context.Background(), event.ProcessCompleted{
		Header: event.NewHeader("child"),
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
	if rec.inputs[1].Event != hooks.SubagentStop || rec.inputs[1].Reason != "subagent completed" {
		t.Fatalf("stop input = %+v", rec.inputs[1])
	}
	if stop.Status != "completed" || stop.Result != "auth failures are handled in middleware" || stop.Description != "inspect auth" {
		t.Fatalf("stop subagent = %+v", stop)
	}
}

func TestSubagentLifecycleProjectsRestoredChildOnStop(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound(
		[]hooks.Hook{{Event: hooks.SubagentStop, Command: "record", Source: "test"}},
		hooks.NewRunner(rec, nil),
	)
	lifecycle := &subagentLifecycle{
		sessionID: "sess",
		cwd:       "/work",
		hooks:     bound,
		project: func(processID string) (agentexec.SubagentProjection, bool) {
			if processID != "restored-child" {
				return agentexec.SubagentProjection{}, false
			}
			return agentexec.SubagentProjection{
				ParentProcessID: "restored-root",
				Description:     "inspect auth",
				Prompt:          "Find where auth failures are handled.",
				Reply:           "auth failures are handled in middleware",
			}, true
		},
	}
	if err := lifecycle.confirmRoot("restored-root"); err != nil {
		t.Fatalf("confirmRoot: %v", err)
	}

	lifecycle.listener("restored-turn").OnEvent(t.Context(), event.ProcessCompleted{
		Header: event.NewHeader("restored-child"),
	})

	if len(rec.inputs) != 1 {
		t.Fatalf("hook inputs = %d, want 1: %#v", len(rec.inputs), rec.inputs)
	}
	got := rec.inputs[0].Subagent
	if got.ProcessID != "restored-child" ||
		got.ParentProcessID != "restored-root" ||
		got.Description != "inspect auth" ||
		got.Prompt != "Find where auth failures are handled." ||
		got.Status != hooks.SubagentCompleted ||
		got.Result != "auth failures are handled in middleware" {
		t.Fatalf("restored stop subagent = %+v", got)
	}
}

func TestSubagentLifecyclePreservesNestedParentage(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.SubagentStart, Command: "record", Source: "test"},
	}, hooks.NewRunner(rec, nil))
	lifecycle := &subagentLifecycle{
		sessionID: "sess",
		cwd:       "/work",
		hooks:     bound,
	}
	listener := lifecycle.listener("nested-turn")

	listener.OnEvent(t.Context(), event.ProcessCreated{Header: event.NewHeader("root")})
	listener.OnEvent(t.Context(), event.ProcessCreated{
		Header:   event.NewHeader("child"),
		ParentID: "root",
	})
	listener.OnEvent(t.Context(), event.ProcessCreated{
		Header:   event.NewHeader("grandchild"),
		ParentID: "child",
	})

	if len(rec.inputs) != 2 {
		t.Fatalf("hook inputs = %d, want child and grandchild", len(rec.inputs))
	}
	if got := rec.inputs[0].Subagent.ParentProcessID; got != "root" {
		t.Fatalf("child parent = %q, want root", got)
	}
	if got := rec.inputs[1].Subagent.ParentProcessID; got != "child" {
		t.Fatalf("grandchild parent = %q, want child", got)
	}
}

func TestSubagentLifecycleRejectsMismatchedReturnedRoot(t *testing.T) {
	lifecycle := &subagentLifecycle{}
	listener := lifecycle.listener("turn")

	listener.OnEvent(t.Context(), event.ProcessCreated{Header: event.NewHeader("root")})
	if err := lifecycle.confirmRoot("other"); err == nil {
		t.Fatal("confirmRoot accepted a returned process that differs from ProcessCreated")
	}
}

func TestSubagentLifecycleExistsOnlyForRelevantHooks(t *testing.T) {
	stopOnly := hooks.NewBound([]hooks.Hook{{Event: hooks.Stop}}, nil)
	if lifecycle := newSubagentLifecycle("session", "/work", stopOnly, nil); lifecycle != nil {
		t.Fatal("installed a subtree listener for unrelated hooks")
	}
	subagent := hooks.NewBound([]hooks.Hook{{Event: hooks.SubagentStart}}, nil)
	if lifecycle := newSubagentLifecycle("session", "/work", subagent, nil); lifecycle == nil {
		t.Fatal("did not install a subtree listener for subagent hooks")
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
	r.inputs = append(r.inputs, req.Input)
	return hooks.CommandResult{}
}
