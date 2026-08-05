package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning/goap"
)

func TestMissingProcessErrorsHaveStableIdentity(t *testing.T) {
	engine, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "kill", run: func() error { return engine.Kill(t.Context(), "proc_missing") }},
		{name: "kill children", run: func() error {
			_, err := engine.KillChildren(t.Context(), "proc_missing")
			return err
		}},
		{name: "resume", run: func() error { return engine.Resume(t.Context(), "proc_missing", "susp_1", true) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrProcessNotFound) {
				t.Fatalf("error = %v, want ErrProcessNotFound", err)
			}
		})
	}
}

func TestAsyncEntryPointsReturnAdmissionErrorsSynchronously(t *testing.T) {
	engine, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	segment, err := engine.Start(t.Context(), nil, core.Bindings{}, core.ProcessOptions{})
	if segment != nil || err == nil {
		t.Fatalf("Start = %#v, %v; want nil segment and error", segment, err)
	}
	segment, err = engine.ContinueAsync(t.Context(), "proc_missing")
	if segment != nil || !errors.Is(err, ErrProcessNotFound) {
		t.Fatalf("ContinueAsync = %#v, %v; want nil segment and ErrProcessNotFound", segment, err)
	}
}

func TestContinuationAdmissionCannotCrossRegistryReservation(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	process := createProcessForTest(
		t,
		engine,
		deploymentFixture("reserved-continuation", core.ConditionSet{"finish": core.True}, nil),
		core.Bindings{},
		core.ProcessOptions{},
	)
	if !engine.processes.reserveProcesses([]*Process{process}) {
		t.Fatal("reserve process")
	}
	if err := engine.Continue(t.Context(), process.ID()); !errors.Is(err, ErrProcessNotFound) {
		t.Fatalf("Continue reserved process = %v, want ErrProcessNotFound", err)
	}
	if err := engine.Resume(t.Context(), process.ID(), "suspension", true); !errors.Is(err, ErrProcessNotFound) {
		t.Fatalf("Resume reserved process = %v, want ErrProcessNotFound", err)
	}
	if got := process.Status(); got != core.StatusNotStarted {
		t.Fatalf("reserved process status = %s, want %s", got, core.StatusNotStarted)
	}

	engine.processes.releaseProcesses([]*Process{process})
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue released process: %v", err)
	}
}

func TestResumeContextBoundsProcessTreeAdmission(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	process := createProcessForTest(
		t,
		engine,
		deploymentFixture("bounded-resume", core.ConditionSet{"finish": core.True}, nil),
		core.Bindings{},
		core.ProcessOptions{},
	)
	release, err := engine.processMutations.acquire(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := engine.Resume(ctx, process.ID(), "suspension", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("Resume = %v, want context cancellation", err)
	}
}

func TestStartClaimsSegmentOwnershipBeforeReturning(t *testing.T) {
	release := make(chan struct{})
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	definition := deploymentFixture("start-ownership", core.ConditionSet{"finish": core.True}, func(*core.ProcessContext) core.ActionStatus {
		<-release
		return core.ActionSucceeded
	})

	segment, err := engine.Start(t.Context(), definition, core.Bindings{}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started, err := segment.Process().state.beginRun(); started || !errors.Is(err, ErrProcessRunning) {
		t.Fatalf("second beginRun = (%v, %v), want false and ErrProcessRunning", started, err)
	}
	close(release)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := segment.Await(ctx); err != nil {
		t.Fatalf("Await: %v", err)
	}
}

type constructorExtension struct{ name string }

func (e *constructorExtension) Name() string { return e.name }
func (*constructorExtension) Check(core.ProcessView) (bool, string) {
	return false, ""
}

type nameOnlyExtension struct{ name string }

func (e nameOnlyExtension) Name() string { return e.name }

func TestNewEngineReturnsConfigErrors(t *testing.T) {
	duplicate := &constructorExtension{name: "duplicate"}
	for _, test := range []struct {
		name     string
		config   Config
		contains string
	}{
		{name: "nil extension", config: Config{Extensions: []core.Extension{nil}}, contains: "nil extension"},
		{name: "typed nil extension", config: Config{Extensions: []core.Extension{(*constructorExtension)(nil)}}, contains: "nil extension"},
		{name: "empty extension name", config: Config{Extensions: []core.Extension{&constructorExtension{}}}, contains: "empty Name"},
		{name: "duplicate extension", config: Config{Extensions: []core.Extension{duplicate, duplicate}}, contains: "already registered"},
		{name: "extension without capability", config: Config{Extensions: []core.Extension{nameOnlyExtension{name: "empty"}}}, contains: "no engine-scoped capability"},
		{name: "negative child depth", config: Config{MaxChildDepth: -1}, contains: "MaxChildDepth"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, err := New(test.config)
			if engine != nil {
				t.Fatalf("engine = %#v, want nil", engine)
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestNewEngineBuildsZeroConfig(t *testing.T) {
	engine, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if engine == nil || engine.events == nil || engine.dependencies == nil {
		t.Fatalf("incomplete engine: %#v", engine)
	}
}

func TestMustNewEnginePanicsOnConfigError(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("MustNew did not panic")
		}
	}()
	MustNew(Config{Extensions: []core.Extension{nil}})
}
