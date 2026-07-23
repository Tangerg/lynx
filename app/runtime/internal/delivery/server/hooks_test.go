package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// fakeHookTrust records the workspace coordinator's trust calls (Trust/Untrust)
// so the hooks delivery handler can be tested against a wired trust store.
type fakeHookTrust struct {
	projectRoot string
	trusted     bool
	calls       int
}

func (f *fakeHookTrust) Trust(_ context.Context, projectRoot string) error {
	f.projectRoot = projectRoot
	f.trusted = true
	f.calls++
	return nil
}

func (f *fakeHookTrust) Untrust(_ context.Context, projectRoot string) error {
	f.projectRoot = projectRoot
	f.trusted = false
	f.calls++
	return nil
}

func serverWithHookTrust(trust workspaceapp.HookTrustStore) *Server {
	return newWorkspaceServerWithConfig("", workspaceTestConfig{Trust: trust})
}

func TestSetHookTrustCanonicalizesProjectRoot(t *testing.T) {
	trust := &fakeHookTrust{}
	s := serverWithHookTrust(trust)
	projectRoot := t.TempDir()

	err := s.SetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot,
		Trusted:     true,
	})
	if err != nil {
		t.Fatalf("setTrust: %v", err)
	}
	if trust.calls != 1 || trust.projectRoot != workspacepath.Canonical(projectRoot) || !trust.trusted {
		t.Fatalf("trusted root=%q trusted=%v calls=%d, want %q true 1", trust.projectRoot, trust.trusted, trust.calls, workspacepath.Canonical(projectRoot))
	}
}

func TestSetHookTrustRejectsUnavailableProjectRoot(t *testing.T) {
	trust := &fakeHookTrust{}
	s := serverWithHookTrust(trust)
	missing := filepath.Join(t.TempDir(), "missing")

	err := s.SetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: missing,
		Trusted:     true,
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("setTrust err = %v, want ErrCwdUnavailable", err)
	}
	if trust.calls != 0 {
		t.Fatalf("trust store calls = %d, want 0", trust.calls)
	}
}

type failingHookInspector struct{ err error }

func (i failingHookInspector) Inspect(context.Context, string) (domainhooks.Inspection, error) {
	return domainhooks.Inspection{}, i.err
}

type staticHookInspector struct{ inspection domainhooks.Inspection }

func (i staticHookInspector) Inspect(context.Context, string) (domainhooks.Inspection, error) {
	return i.inspection, nil
}

func TestListHooksPreservesCompleteHookDefinition(t *testing.T) {
	root := t.TempDir()
	s := newWorkspaceServerWithConfig(root, workspaceTestConfig{Hooks: staticHookInspector{inspection: domainhooks.Inspection{
		ProjectRoot: root,
		Hooks: []domainhooks.Hook{{
			Event: domainhooks.SubagentStart, Command: "audit", TimeoutMs: 2500,
			Scope: domainhooks.ScopeGlobal, Source: "/home/user/.lyra/hooks.json",
		}},
	}}})

	result, err := s.ListHooks(t.Context(), protocol.ListHooksRequest{})
	if err != nil {
		t.Fatalf("ListHooks: %v", err)
	}
	if len(result.Hooks) != 1 {
		t.Fatalf("hooks = %+v, want one", result.Hooks)
	}
	hook := result.Hooks[0]
	if hook.Event != protocol.HookEventSubagentStart || hook.TimeoutMs != 2500 || !hook.Active {
		t.Fatalf("hook = %+v, want complete active subagent hook", hook)
	}
}

func TestListHooksPreservesInspectionFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	root := t.TempDir()
	s := newWorkspaceServerWithConfig(root, workspaceTestConfig{Hooks: failingHookInspector{err: wantErr}})

	if _, err := s.ListHooks(context.Background(), protocol.ListHooksRequest{}); !errors.Is(err, wantErr) {
		t.Fatalf("ListHooks error = %v, want %v", err, wantErr)
	}
}
