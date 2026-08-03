package turn

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
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
		childRun: childRunLookup(runs.ChildRunBinding{
			ProcessID: "child", RunID: "run-child", ParentRunID: "run-root",
		}),
		project: func(processID string) (agentexec.SubagentProjection, bool) {
			if processID == "child" {
				return agentexec.SubagentProjection{
					Summary:      "inspect auth",
					Instructions: "Find where auth failures are handled.",
					Reply:        "auth failures are handled in middleware",
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
	if rec.inputs[0].Event != hooks.SubagentStart || start.RunID != "run-child" || start.ParentRunID != "run-root" {
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
		childRun: childRunLookup(runs.ChildRunBinding{
			ProcessID: "restored-child", RunID: "run-restored-child", ParentRunID: "run-restored-root",
		}),
		project: func(processID string) (agentexec.SubagentProjection, bool) {
			if processID != "restored-child" {
				return agentexec.SubagentProjection{}, false
			}
			return agentexec.SubagentProjection{
				Summary:      "inspect auth",
				Instructions: "Find where auth failures are handled.",
				Reply:        "auth failures are handled in middleware",
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
	if got.RunID != "run-restored-child" ||
		got.ParentRunID != "run-restored-root" ||
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
		childRun: childRunLookup(
			runs.ChildRunBinding{ProcessID: "child", RunID: "run-child", ParentRunID: "run-root"},
			runs.ChildRunBinding{ProcessID: "grandchild", RunID: "run-grandchild", ParentRunID: "run-child"},
		),
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
	if got := rec.inputs[0].Subagent.ParentRunID; got != "run-root" {
		t.Fatalf("child parent Run = %q, want run-root", got)
	}
	if got := rec.inputs[1].Subagent.ParentRunID; got != "run-child" {
		t.Fatalf("grandchild parent Run = %q, want run-child", got)
	}
}

func TestSubagentLifecycleIgnoresFrameworkInternalChild(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound(
		[]hooks.Hook{{Event: hooks.SubagentStart, Command: "record", Source: "test"}},
		hooks.NewRunner(rec, nil),
	)
	lifecycle := &subagentLifecycle{
		rootID: "root", sessionID: "sess", cwd: "/work", hooks: bound,
		childRun: childRunLookup(),
	}
	lifecycle.listener("turn").OnEvent(t.Context(), event.ProcessCreated{
		Header: event.NewHeader("internal-child"), ParentID: "root",
	})
	if len(rec.inputs) != 0 {
		t.Fatalf("Framework-internal child leaked into product hooks: %#v", rec.inputs)
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
	if lifecycle := newSubagentLifecycle("session", "/work", stopOnly, nil, nil); lifecycle != nil {
		t.Fatal("installed a subtree listener for unrelated hooks")
	}
	subagent := hooks.NewBound([]hooks.Hook{{Event: hooks.SubagentStart}}, nil)
	if lifecycle := newSubagentLifecycle("session", "/work", subagent, nil, nil); lifecycle == nil {
		t.Fatal("did not install a subtree listener for subagent hooks")
	}
}

func childRunLookup(bindings ...runs.ChildRunBinding) func(string) (runs.ChildRunBinding, bool) {
	byProcess := make(map[string]runs.ChildRunBinding, len(bindings))
	for _, binding := range bindings {
		byProcess[binding.ProcessID] = binding
	}
	return func(processID string) (runs.ChildRunBinding, bool) {
		binding, ok := byProcess[processID]
		return binding, ok
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
