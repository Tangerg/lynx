package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRuntimeInspectHooksReturnsEmptyWhenUnconfigured(t *testing.T) {
	c := New(Config{Paths: testPaths{}})

	got, err := c.InspectHooks(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("InspectHooks: %v", err)
	}
	if got.ProjectRoot != "" || got.ProjectTrusted || len(got.Hooks) != 0 {
		t.Fatalf("InspectHooks = %+v, want empty inspection", got)
	}
}

func TestRuntimeInspectHooksUsesInspectionPort(t *testing.T) {
	inspector := &fakeHookInspector{
		inspection: hooks.Inspection{
			ProjectRoot:    "/repo",
			ProjectTrusted: true,
			Hooks: []hooks.Hook{{
				Event:   hooks.UserPromptSubmit,
				Command: "make test",
			}},
		},
	}
	c := New(Config{Paths: testPaths{}, Hooks: inspector})

	got, err := c.InspectHooks(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("InspectHooks: %v", err)
	}
	if inspector.cwd != "/repo" {
		t.Fatalf("inspect cwd = %q, want /repo", inspector.cwd)
	}
	if got.ProjectRoot != "/repo" || !got.ProjectTrusted || len(got.Hooks) != 1 || got.Hooks[0].Command != "make test" {
		t.Fatalf("InspectHooks = %+v", got)
	}
}

type fakeHookInspector struct {
	cwd        string
	inspection hooks.Inspection
	err        error
}

func (i *fakeHookInspector) Inspect(_ context.Context, cwd string) (hooks.Inspection, error) {
	i.cwd = cwd
	return i.inspection, i.err
}

func TestRuntimeInspectHooksPreservesInspectorFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	c := New(Config{Paths: testPaths{}, Hooks: &fakeHookInspector{err: wantErr}})

	if _, err := c.InspectHooks(context.Background(), "/repo"); !errors.Is(err, wantErr) {
		t.Fatalf("InspectHooks error = %v, want %v", err, wantErr)
	}
}
