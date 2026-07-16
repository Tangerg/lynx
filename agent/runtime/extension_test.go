package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// orderRecorder is the test fixture that captures the visit order of
// onion / wrap chains. Multiple extension types append to its log so a
// single test asserts both the dispatch reach and the relative ordering.
type orderRecorder struct {
	mu  sync.Mutex
	log []string
}

func (r *orderRecorder) record(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, label)
}

func (r *orderRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.log))
	copy(out, r.log)
	return out
}

// orderedInterceptor records its own enter / exit around next().
type orderedInterceptor struct {
	name     string
	recorder *orderRecorder
}

func (i orderedInterceptor) Name() string { return i.name }

func (i orderedInterceptor) RunAction(_ context.Context, _ core.ProcessView, _ core.Action, next func() (core.ActionStatus, error)) (core.ActionStatus, error) {
	i.recorder.record(i.name + ":enter")
	status, err := next()
	i.recorder.record(i.name + ":exit")
	return status, err
}

// TestEngineExtensionDedupPanic — boot-time configuration error must
// not silently overwrite; duplicate Name within the engine layer
// panics.
func TestEngineExtensionDedupPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate extension Name")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "already registered") {
			t.Fatalf("panic message = %q, want substring %q", msg, "already registered")
		}
	}()

	rec := &orderRecorder{}
	agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{
			orderedInterceptor{name: "dup", recorder: rec},
			orderedInterceptor{name: "dup", recorder: rec},
		},
	})
}

// TestEngineExtensionEmptyNamePanic — empty Name is a misconfiguration.
func TestEngineExtensionEmptyNamePanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on empty extension Name")
		}
	}()

	agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{orderedInterceptor{name: "", recorder: &orderRecorder{}}},
	})
}

// TestActionMiddlewareOnionOrdering verifies engine actionMiddleware form
// the outer onion and process actionMiddleware sit inside.
func TestActionMiddlewareOnionOrdering(t *testing.T) {
	type runIn struct{ V int }
	type runOut struct{ V int }

	rec := &orderRecorder{}
	a := agent.New(agent.AgentConfig{Name: "actionMiddleware", Actions: []agent.Action{agent.NewAction("step", func(_ context.Context, _ *core.ProcessContext, in runIn) (runOut, error) {
		rec.record("body")
		return runOut{V: in.V + 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[runOut](core.GoalConfig{Description: "out"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{
			orderedInterceptor{name: "engine-A", recorder: rec},
			orderedInterceptor{name: "engine-B", recorder: rec},
		},
	})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	proc, err := engine.Run(
		t.Context(), a,
		map[string]any{core.DefaultBindingName: runIn{V: 1}},
		core.ProcessOptions{
			Extensions: []core.Extension{
				orderedInterceptor{name: "process-X", recorder: rec},
				orderedInterceptor{name: "process-Y", recorder: rec},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	want := []string{
		"engine-A:enter",
		"engine-B:enter",
		"process-X:enter",
		"process-Y:enter",
		"body",
		"process-Y:exit",
		"process-X:exit",
		"engine-B:exit",
		"engine-A:exit",
	}
	got := rec.snapshot()
	if len(got) != len(want) {
		t.Fatalf("log = %v\nwant = %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("log[%d] = %q, want %q\nfull = %v", i, got[i], want[i], got)
		}
	}
}

// failingValidator is the simplest AgentValidator extension — always
// rejects with the supplied error so tests can assert routing.
type failingValidator struct {
	name string
	err  error
}

func (v failingValidator) Name() string                 { return v.name }
func (v failingValidator) Validate(_ *core.Agent) error { return v.err }

// TestAgentValidatorRejectsDeploy — extension can veto Deploy, error is
// attributed to the validator's Name.
func TestAgentValidatorRejectsDeploy(t *testing.T) {
	type vIn struct{}
	type vOut struct{}
	a := agent.New(agent.AgentConfig{Name: "validated", Actions: []agent.Action{agent.NewAction("op", func(_ context.Context, _ *core.ProcessContext, _ vIn) (vOut, error) {
		return vOut{}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[vOut](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{
			failingValidator{name: "policy", err: errors.New("missing SLA tag")},
		},
	})
	_, err := engine.Deploy(a)
	if err == nil {
		t.Fatal("expected validator to reject Deploy")
	}
	if !strings.Contains(err.Error(), `validator "policy"`) || !strings.Contains(err.Error(), "missing SLA tag") {
		t.Fatalf("error = %v, want validator name and message", err)
	}
}

// TestDeploy_ReportsAllProblems confirms the multi-layer validation
// aggregates every problem (two unreachable goal conditions + a failing
// validator) into one error rather than failing on the first.
func TestDeploy_ReportsAllProblems(t *testing.T) {
	type pIn struct{}
	type pOut struct{}
	a := agent.New(agent.AgentConfig{Name: "multi-problem", Actions: []agent.Action{agent.NewAction("step", func(_ context.Context, _ *core.ProcessContext, _ pIn) (pOut, error) {
		return pOut{}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[pOut](core.GoalConfig{Description: "needs missing conditions", Preconditions: []string{"never_a", "never_b"}})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{
			failingValidator{name: "policy", err: errors.New("missing SLA tag")},
		},
	})

	_, err := engine.Deploy(a)
	if err == nil {
		t.Fatal("expected deploy to fail")
	}
	msg := err.Error()
	for _, want := range []string{"never_a", "never_b", `validator "policy"`, "missing SLA tag"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q; full error:\n%s", want, msg)
		}
	}
}

// vetoApprover blocks every goal it sees.
type vetoApprover struct{ name string }

func (v vetoApprover) Name() string                                { return v.name }
func (vetoApprover) Approve(_ core.ProcessView, _ *core.Goal) bool { return false }

// TestGoalApproverVetoesPlan — when an approver vetoes the only goal,
// the planner sees no goals → process ends Stuck.
func TestGoalApproverVetoesPlan(t *testing.T) {
	type vetoIn struct{}
	type vetoOut struct{}
	a := agent.New(agent.AgentConfig{Name: "vetoed", Actions: []agent.Action{agent.NewAction("op", func(_ context.Context, _ *core.ProcessContext, _ vetoIn) (vetoOut, error) {
		return vetoOut{}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[vetoOut](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{vetoApprover{name: "veto"}},
	})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	proc, err := engine.Run(t.Context(), a, nil, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusStuck {
		t.Fatalf("status = %s, want Stuck", proc.Status())
	}
}

// TestProcessExtensionDedupErrors — duplicate Name inside a single
// ProcessOptions.Extensions surfaces as a returned error (not a panic
// — process creation is request-time, not boot-time).
func TestProcessExtensionDedupErrors(t *testing.T) {
	type dIn struct{}
	type dOut struct{}
	a := agent.New(agent.AgentConfig{Name: "proc-dup", Actions: []agent.Action{agent.NewAction("op", func(_ context.Context, _ *core.ProcessContext, _ dIn) (dOut, error) {
		return dOut{}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[dOut](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatal(err)
	}
	rec := &orderRecorder{}
	_, err := engine.Run(t.Context(), a, nil, core.ProcessOptions{
		Extensions: []core.Extension{
			orderedInterceptor{name: "same", recorder: rec},
			orderedInterceptor{name: "same", recorder: rec},
		},
	})
	if err == nil {
		t.Fatal("expected error on duplicate process-scope extension Name")
	}
	if !strings.Contains(err.Error(), `duplicate name "same"`) {
		t.Fatalf("error = %v, want duplicate-name detail", err)
	}
}

// processOnlyListener counts events with a process id.
type processOnlyListener struct {
	name  string
	count *int
}

func (l processOnlyListener) Name() string { return l.name }
func (l processOnlyListener) OnEvent(_ context.Context, e event.Event) {
	if e.ProcessID() != "" {
		*l.count++
	}
}

// TestProcessScopedListenerOnlyForOwnProcess — a per-process
// EventListener registered via ProcessOptions.Extensions sees its own
// process events. (We can't easily test "doesn't see other processes"
// here without a multi-process fixture; the assertion narrows to "fires
// at all".)
func TestProcessScopedListenerFires(t *testing.T) {
	type pOut struct{ V int }
	a := agent.New(agent.AgentConfig{Name: "proc-listener", Actions: []agent.Action{agent.NewAction("op", func(_ context.Context, _ *core.ProcessContext, in string) (pOut, error) {
		return pOut{V: len(in)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[pOut](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatal(err)
	}
	count := 0
	proc, err := engine.Run(
		t.Context(), a,
		map[string]any{core.DefaultBindingName: "hello"},
		core.ProcessOptions{
			Extensions: []core.Extension{
				processOnlyListener{name: "proc-listener", count: &count},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure=%v", proc.Status(), proc.Failure())
	}
	if count == 0 {
		t.Fatal("process-scope listener received no events")
	}
}
