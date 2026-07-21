package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
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
		{name: "remove", run: func() error { return engine.Remove("proc_missing") }},
		{name: "resume", run: func() error { return engine.Resume("proc_missing", "susp_1", true) }},
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
	process, done, err := engine.Start(t.Context(), nil, core.Bindings{}, core.ProcessOptions{})
	if process != nil || done != nil || err == nil {
		t.Fatalf("Start = %#v, %#v, %v; want nil process, nil channel, error", process, done, err)
	}
	done, err = engine.ContinueAsync(t.Context(), "proc_missing")
	if done != nil || !errors.Is(err, ErrProcessNotFound) {
		t.Fatalf("ContinueAsync = %#v, %v; want nil channel and ErrProcessNotFound", done, err)
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
	var processStore *storetest.MemoryProcessStore
	var sessionStore *storetest.MemorySessionStore
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
		{name: "whitespace build id", config: Config{BuildID: " build "}, contains: "BuildID"},
		{name: "auto snapshot without store", config: Config{AutoSnapshot: true}, contains: "requires ProcessStore"},
		{name: "negative snapshot finalize timeout", config: Config{SnapshotFinalizeTimeout: -1}, contains: "SnapshotFinalizeTimeout"},
		{name: "negative child depth", config: Config{MaxChildDepth: -1}, contains: "MaxChildDepth"},
		{name: "typed nil process store", config: Config{ProcessStore: processStore}, contains: "ProcessStore is typed nil"},
		{name: "typed nil session store", config: Config{SessionStore: sessionStore}, contains: "SessionStore is typed nil"},
		{name: "typed nil child session store", config: Config{ChildSessionStore: sessionStore}, contains: "ChildSessionStore is typed nil"},
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
