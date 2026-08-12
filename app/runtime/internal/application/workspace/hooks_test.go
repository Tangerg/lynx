package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	apphooks "github.com/Tangerg/lynx/app/runtime/internal/application/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRuntimeInspectReturnsEmptyWhenUnconfigured(t *testing.T) {
	c := NewHooks(NewScope("", "", testPaths{}), nil, nil, nil)

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
		inspection: apphooks.Inspection{
			ProjectRoot:    "/repo",
			ProjectTrusted: true,
			Hooks: []hooks.Hook{{
				Event:   hooks.UserPromptSubmit,
				Command: "make test",
			}},
		},
	}
	c := NewHooks(NewScope("", "", testPaths{}), inspector, nil, nil)

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
	inspection apphooks.Inspection
	err        error
}

func (i *fakeHookInspector) Inspect(_ context.Context, cwd string) (apphooks.Inspection, error) {
	i.cwd = cwd
	return i.inspection, i.err
}

func TestRuntimeInspectPreservesInspectorFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	c := NewHooks(NewScope("", "", testPaths{}), &fakeHookInspector{err: wantErr}, nil, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); !errors.Is(err, wantErr) {
		t.Fatalf("Inspect error = %v, want %v", err, wantErr)
	}
}

func TestHookTrustPublishesOnlyCommittedChanges(t *testing.T) {
	trust := &fakeHookTrust{}
	var notices []invalidation.Notice
	hooks := NewHooks(
		NewScope("", "", testPaths{}), nil, trust,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	if err := hooks.SetProjectTrust(t.Context(), "/repo", true); err != nil {
		t.Fatal(err)
	}
	if trust.trusted != "/repo" || !reflect.DeepEqual(notices, []invalidation.Notice{{Resource: invalidation.Hooks}}) {
		t.Fatalf("trust=%q invalidations=%+v", trust.trusted, notices)
	}

	trust.err = errors.New("write failed")
	if err := hooks.SetProjectTrust(t.Context(), "/repo", false); !errors.Is(err, trust.err) {
		t.Fatalf("SetProjectTrust err = %v, want %v", err, trust.err)
	}
	if len(notices) != 1 {
		t.Fatalf("failed mutation published %+v", notices)
	}
}

type fakeHookTrust struct {
	trusted   string
	untrusted string
	err       error
}

func (s *fakeHookTrust) Trust(_ context.Context, root string) error {
	s.trusted = root
	return s.err
}

func (s *fakeHookTrust) Untrust(_ context.Context, root string) error {
	s.untrusted = root
	return s.err
}
