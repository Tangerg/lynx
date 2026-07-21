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

type actionMiddlewareFunc struct {
	name string
	run  func(func() (core.ActionStatus, error)) (core.ActionStatus, error)
}

type panickingNameExtension struct{ cause error }

func (e panickingNameExtension) Name() string { panic(e.cause) }

type testIDGenerator struct {
	name string
	next func() string
}

func (g testIDGenerator) Name() string { return g.name }
func (g testIDGenerator) Next() string { return g.next() }

type testBlackboard struct {
	core.Blackboard
	name     string
	clone    func() core.Blackboard
	bind     func(any)
	snapshot func() (runtime.BlackboardState, error)
	restore  func(runtime.BlackboardState) error
}

func (b *testBlackboard) Name() string { return b.name }
func (b *testBlackboard) Clone() core.Blackboard {
	if b.clone != nil {
		return b.clone()
	}
	return b.Blackboard.Clone()
}
func (b *testBlackboard) Bind(value any) {
	if b.bind != nil {
		b.bind(value)
		return
	}
	b.Blackboard.Bind(value)
}
func (b *testBlackboard) Snapshot() (runtime.BlackboardState, error) {
	if b.snapshot != nil {
		return b.snapshot()
	}
	return b.Blackboard.(runtime.BlackboardSnapshotter).Snapshot()
}
func (b *testBlackboard) Restore(state runtime.BlackboardState) error {
	if b.restore != nil {
		return b.restore(state)
	}
	return b.Blackboard.(runtime.BlackboardRestorer).Restore(state)
}

func (m actionMiddlewareFunc) Name() string { return m.name }
func (m actionMiddlewareFunc) RunAction(_ context.Context, _ core.ProcessView, _ core.Action, next func() (core.ActionStatus, error)) (core.ActionStatus, error) {
	return m.run(next)
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

func TestEngineExtensionNamePanicReturnsError(t *testing.T) {
	cause := errors.New("name sentinel")
	_, err := runtime.New(runtime.Config{Extensions: []core.Extension{panickingNameExtension{cause: cause}}})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "Name panicked") {
		t.Fatalf("New error = %v, want attributed Name panic", err)
	}
}

func TestEngineFreezesExtensionNameAtRegistration(t *testing.T) {
	middleware := &actionMiddlewareFunc{
		name: "registered-name",
		run: func(func() (core.ActionStatus, error)) (core.ActionStatus, error) {
			panic("middleware failure")
		},
	}
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{middleware}})
	middleware.name = "mutated-name"

	a := extensionBoundaryAgent()
	process, err := engine.Run(t.Context(), a, core.Input("input"), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	failure := process.Failure()
	if failure == nil || !strings.Contains(failure.Error(), `action middleware "registered-name" panicked`) {
		t.Fatalf("failure = %v, want frozen extension name", failure)
	}
	if strings.Contains(failure.Error(), "mutated-name") {
		t.Fatalf("failure used mutable extension name: %v", failure)
	}
}

func extensionBoundaryAgent() *core.Agent {
	type output struct{ Value string }
	return agent.New(agent.AgentConfig{
		Name: "extension-boundary",
		Actions: []agent.Action{agent.NewAction("work", func(context.Context, *core.ProcessContext, string) (output, error) {
			return output{Value: "done"}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
	})
}

func TestIDGeneratorFailuresReturnProcessCreationErrors(t *testing.T) {
	cause := errors.New("ID generator sentinel")
	tests := []struct {
		name      string
		next      func() string
		wantError error
		contains  string
	}{
		{
			name: "panic",
			next: func() string {
				panic(cause)
			},
			wantError: cause,
			contains:  `ID generator "test-id" Next panicked`,
		},
		{name: "empty", next: func() string { return "" }, wantError: runtime.ErrProcessIdentity, contains: "returned"},
		{name: "surrounding whitespace", next: func() string { return " process-1 " }, wantError: runtime.ErrProcessIdentity, contains: "returned"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{testIDGenerator{
				name: "test-id",
				next: test.next,
			}}})
			process, err := engine.Run(t.Context(), extensionBoundaryAgent(), core.Input("input"), core.ProcessOptions{})
			if process != nil || !errors.Is(err, test.wantError) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run = %#v, %v; want nil process and %v containing %q", process, err, test.wantError, test.contains)
			}
		})
	}
}

func TestDuplicateGeneratedProcessIDDoesNotReplaceRegistryEntry(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{testIDGenerator{
		name: "static-id",
		next: func() string { return "process-1" },
	}}})
	definition := extensionBoundaryAgent()
	first, err := engine.Run(t.Context(), definition, core.Input("first"), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := engine.Run(t.Context(), definition, core.Input("second"), core.ProcessOptions{})
	if second != nil || !errors.Is(err, runtime.ErrProcessIdentity) {
		t.Fatalf("second Run = %#v, %v; want duplicate identity error", second, err)
	}
	registered, ok := engine.Process(first.ID())
	if !ok || registered != first {
		t.Fatalf("registered process = %#v, %v; want original %#v", registered, ok, first)
	}
}

func TestBlackboardConstructionFailuresReturnErrors(t *testing.T) {
	baseEngine := agent.MustNewEngine(runtime.Config{})
	base, err := baseEngine.NewBlackboard()
	if err != nil {
		t.Fatalf("NewBlackboard baseline: %v", err)
	}
	cause := errors.New("blackboard sentinel")
	tests := []struct {
		name      string
		prototype *testBlackboard
		wantError error
		contains  string
	}{
		{
			name: "clone panic",
			prototype: &testBlackboard{Blackboard: base, name: "panic-board", clone: func() core.Blackboard {
				panic(cause)
			}},
			wantError: cause,
			contains:  `blackboard "panic-board" Clone panicked`,
		},
		{
			name:      "nil clone",
			prototype: &testBlackboard{Blackboard: base, name: "nil-board", clone: func() core.Blackboard { return nil }},
			contains:  `blackboard "nil-board" Clone returned nil`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{test.prototype}})
			blackboard, err := engine.NewBlackboard()
			if blackboard != nil || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("NewBlackboard = %#v, %v; want nil and %q", blackboard, err, test.contains)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("NewBlackboard error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func TestBlackboardSeedPanicReturnsProcessCreationError(t *testing.T) {
	baseEngine := agent.MustNewEngine(runtime.Config{})
	base, err := baseEngine.NewBlackboard()
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("seed sentinel")
	blackboard := &testBlackboard{
		Blackboard: base,
		name:       "seed-board",
		bind: func(any) {
			panic(cause)
		},
	}
	engine := agent.MustNewEngine(runtime.Config{})
	process, err := engine.Run(t.Context(), extensionBoundaryAgent(), core.Input("input"), core.ProcessOptions{Blackboard: blackboard})
	if process != nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), `blackboard "seed-board" seed panicked`) {
		t.Fatalf("Run = %#v, %v; want attributed seed panic", process, err)
	}
}

func TestBlackboardPersistencePanicsReturnErrors(t *testing.T) {
	baseEngine := agent.MustNewEngine(runtime.Config{})
	base, err := baseEngine.NewBlackboard()
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("blackboard persistence sentinel")
	definition := extensionBoundaryAgent()
	snapshotBoard := &testBlackboard{
		Blackboard: base,
		name:       "snapshot-board",
		snapshot: func() (runtime.BlackboardState, error) {
			panic(cause)
		},
	}
	process, err := baseEngine.Run(t.Context(), definition, core.Input("input"), core.ProcessOptions{Blackboard: snapshotBoard})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := process.Snapshot(); !errors.Is(err, cause) || !strings.Contains(err.Error(), `blackboard "snapshot-board" Snapshot panicked`) {
		t.Fatalf("Snapshot error = %v, want attributed panic", err)
	}

	validProcess, err := baseEngine.Run(t.Context(), definition, core.Input("input"), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("valid Run: %v", err)
	}
	snapshot, err := validProcess.Snapshot()
	if err != nil {
		t.Fatalf("valid Snapshot: %v", err)
	}
	restoreBoard := &testBlackboard{
		Blackboard: base,
		name:       "restore-board",
		restore: func(runtime.BlackboardState) error {
			panic(cause)
		},
	}
	restoreBoard.clone = func() core.Blackboard { return restoreBoard }
	restoreEngine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{restoreBoard}})
	if _, err := restoreEngine.Deploy(t.Context(), definition); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := restoreEngine.RestoreSnapshot(snapshot, core.ProcessOptions{}); !errors.Is(err, cause) || !strings.Contains(err.Error(), `blackboard "restore-board" Restore panicked`) {
		t.Fatalf("RestoreSnapshot error = %v, want attributed panic", err)
	}
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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	proc, err := engine.Run(
		t.Context(), a,
		core.Input(runIn{V: 1}),
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

func TestActionMiddlewareShortCircuitProducesDurableHistory(t *testing.T) {
	type output struct{}
	a := agent.New(agent.AgentConfig{
		Name: "middleware-short-circuit",
		Actions: []agent.Action{agent.NewAction("work", func(context.Context, *core.ProcessContext, struct{}) (output, error) {
			return output{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
	})
	middlewareErr := errors.New("circuit open")
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{actionMiddlewareFunc{
		name: "short-circuit",
		run: func(func() (core.ActionStatus, error)) (core.ActionStatus, error) {
			return core.ActionFailed, middlewareErr
		},
	}}})
	process, err := engine.Run(t.Context(), a, core.Input(struct{}{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !errors.Is(process.Failure(), middlewareErr) {
		t.Fatalf("failure = %v, want middleware error", process.Failure())
	}
	if history := process.History(); len(history) != 1 {
		t.Fatalf("history = %#v, want one action run", history)
	}
	if _, err := process.Snapshot(); err != nil {
		t.Fatalf("Snapshot after short circuit: %v", err)
	}
}

func TestActionMiddlewareNextRunsAtMostOnce(t *testing.T) {
	type output struct{}
	bodyRuns := 0
	a := agent.New(agent.AgentConfig{
		Name: "middleware-next-once",
		Actions: []agent.Action{agent.NewAction("work", func(context.Context, *core.ProcessContext, struct{}) (output, error) {
			bodyRuns++
			return output{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
	})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{actionMiddlewareFunc{
		name: "double-next",
		run: func(next func() (core.ActionStatus, error)) (core.ActionStatus, error) {
			firstStatus, firstErr := next()
			secondStatus, secondErr := next()
			if firstStatus != secondStatus || !errors.Is(secondErr, firstErr) {
				return core.ActionFailed, errors.New("next returned inconsistent results")
			}
			return firstStatus, firstErr
		},
	}}})
	process, err := engine.Run(t.Context(), a, core.Input(struct{}{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusCompleted || bodyRuns != 1 {
		t.Fatalf("status/body runs = %s/%d, want completed/1", process.Status(), bodyRuns)
	}
}

func TestActionMiddlewarePanicFailsProcess(t *testing.T) {
	type output struct{}
	cause := errors.New("middleware panic")
	a := agent.New(agent.AgentConfig{
		Name: "middleware-panic",
		Actions: []agent.Action{agent.NewAction("work", func(context.Context, *core.ProcessContext, struct{}) (output, error) {
			return output{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
	})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{actionMiddlewareFunc{
		name: "panic",
		run: func(func() (core.ActionStatus, error)) (core.ActionStatus, error) {
			panic(cause)
		},
	}}})
	process, err := engine.Run(t.Context(), a, core.Input(struct{}{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), cause) {
		t.Fatalf("status/failure = %s/%v", process.Status(), process.Failure())
	}
	if !strings.Contains(process.Failure().Error(), `action middleware "panic" panicked`) {
		t.Fatalf("failure = %v, want middleware panic context", process.Failure())
	}
}

// failingValidator is the simplest AgentValidator extension — always
// rejects with the supplied error so tests can assert routing.
type failingValidator struct {
	name string
	err  error
}

type capturingValidator struct {
	seen *core.Agent
}

func (*capturingValidator) Name() string { return "capture" }
func (v *capturingValidator) Validate(agent *core.Agent) error {
	v.seen = agent
	return nil
}

type panickingValidator struct{ cause error }

func (panickingValidator) Name() string                 { return "panicking-validator" }
func (v panickingValidator) Validate(*core.Agent) error { panic(v.cause) }

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
	_, err := engine.Deploy(t.Context(), a)
	if err == nil {
		t.Fatal("expected validator to reject Deploy")
	}
	if !strings.Contains(err.Error(), `validator "policy"`) || !strings.Contains(err.Error(), "missing SLA tag") {
		t.Fatalf("error = %v, want validator name and message", err)
	}
}

func TestAgentValidatorReceivesCompiledSnapshot(t *testing.T) {
	definition := newExtensionTestAgent()
	validator := new(capturingValidator)
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{validator}})
	deployment, err := engine.Deploy(t.Context(), definition)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if validator.seen == nil || validator.seen == definition || validator.seen != deployment.Agent() {
		t.Fatalf("validator agent = %p, source = %p, deployment = %p", validator.seen, definition, deployment.Agent())
	}
}

func TestAgentValidatorPanicRejectsDeploy(t *testing.T) {
	cause := errors.New("validator sentinel")
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{panickingValidator{cause: cause}}})
	_, err := engine.Deploy(t.Context(), newExtensionTestAgent())
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "agent validator panicked") {
		t.Fatalf("Deploy error = %v, want wrapped validator panic", err)
	}
}

func newExtensionTestAgent() *core.Agent {
	type input struct{}
	type output struct{}
	return agent.New(agent.AgentConfig{
		Name: "validated-snapshot",
		Actions: []agent.Action{agent.NewAction("op", func(context.Context, *core.ProcessContext, input) (output, error) {
			return output{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
	})
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

	_, err := engine.Deploy(t.Context(), a)
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

type panickingApprover struct{ cause error }

func (panickingApprover) Name() string { return "panic-approver" }
func (a panickingApprover) Approve(core.ProcessView, *core.Goal) bool {
	panic(a.cause)
}

type panickingStopPolicy struct{ cause error }

func (panickingStopPolicy) Name() string { return "panic-stop" }
func (p panickingStopPolicy) Check(core.ProcessView) (bool, string) {
	panic(p.cause)
}

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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	proc, err := engine.Run(t.Context(), a, core.Bindings{}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusStuck {
		t.Fatalf("status = %s, want Stuck", proc.Status())
	}
}

func TestExtensionDecisionPanicsFailProcess(t *testing.T) {
	for _, test := range []struct {
		name      string
		extension core.Extension
		contains  string
	}{
		{name: "goal approver", extension: panickingApprover{cause: errors.New("approver sentinel")}, contains: `goal approver "panic-approver" panicked`},
		{name: "stop policy", extension: panickingStopPolicy{cause: errors.New("stop sentinel")}, contains: `stop policy "panic-stop" panicked`},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := newExtensionTestAgent()
			engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{test.extension}})
			process, err := engine.Run(t.Context(), definition, core.Input(struct{}{}), core.ProcessOptions{})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if process.Status() != core.StatusFailed || !strings.Contains(process.Failure().Error(), test.contains) {
				t.Fatalf("status/failure = %s/%v, want attributed extension panic", process.Status(), process.Failure())
			}
		})
	}
}

func TestConditionPanicFailsObservation(t *testing.T) {
	type output struct{}
	cause := errors.New("condition sentinel")
	definition := agent.New(agent.AgentConfig{
		Name: "panic-condition",
		Actions: []agent.Action{agent.NewAction("work", func(context.Context, *core.ProcessContext, struct{}) (output, error) {
			return output{}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})},
		Conditions: []core.Condition{core.NewCondition("reachable", func(context.Context, *core.ConditionEnv) core.Truth {
			panic(cause)
		})},
	})
	engine := agent.MustNewEngine(runtime.Config{})

	process, err := engine.Run(t.Context(), definition, core.Input(struct{}{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), cause) {
		t.Fatalf("status/failure = %s/%v, want condition failure", process.Status(), process.Failure())
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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	rec := &orderRecorder{}
	_, err := engine.Run(t.Context(), a, core.Bindings{}, core.ProcessOptions{
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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	count := 0
	proc, err := engine.Run(
		t.Context(), a,
		core.Input("hello"),
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
