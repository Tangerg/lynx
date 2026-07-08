package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRuntimeInspectHooksReturnsEmptyWhenUnconfigured(t *testing.T) {
	rt := &Runtime{}

	got := rt.InspectHooks(context.Background(), "/repo")
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
	rt := &Runtime{hookInspection: inspector}

	got := rt.InspectHooks(context.Background(), "/repo")
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
}

func (i *fakeHookInspector) Inspect(_ context.Context, cwd string) hooks.Inspection {
	i.cwd = cwd
	return i.inspection
}
