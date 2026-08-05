package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRuntimeInspectReturnsEmptyWhenUnconfigured(t *testing.T) {
	c := NewHooks(NewScope("", "", testPaths{}), nil, nil)

	got, err := c.Inspect(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.ProjectRoot != "" || got.ProjectTrusted || len(got.Hooks) != 0 {
		t.Fatalf("Inspect = %+v, want empty inspection", got)
	}
}

func TestRuntimeInspectUsesInspectionPort(t *testing.T) {
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
	c := NewHooks(NewScope("", "", testPaths{}), inspector, nil)

	got, err := c.Inspect(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if inspector.cwd != "/repo" {
		t.Fatalf("inspect cwd = %q, want /repo", inspector.cwd)
	}
	if got.ProjectRoot != "/repo" || !got.ProjectTrusted || len(got.Hooks) != 1 || got.Hooks[0].Hook.Command != "make test" || !got.Hooks[0].Active {
		t.Fatalf("Inspect = %+v", got)
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

func TestRuntimeInspectPreservesInspectorFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	c := NewHooks(NewScope("", "", testPaths{}), &fakeHookInspector{err: wantErr}, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); !errors.Is(err, wantErr) {
		t.Fatalf("Inspect error = %v, want %v", err, wantErr)
	}
}
