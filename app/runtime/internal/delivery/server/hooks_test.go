package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

type hookTrustRuntime struct {
	projectRoot string
	trusted     bool
	calls       int
}

func (r *hookTrustRuntime) InspectHooks(context.Context, string) hooks.Inspection {
	return hooks.Inspection{}
}

func (r *hookTrustRuntime) SetProjectHookTrust(_ context.Context, projectRoot string, trusted bool) error {
	r.projectRoot = projectRoot
	r.trusted = trusted
	r.calls++
	return nil
}

func TestWorkspaceSetHookTrustCanonicalizesProjectRoot(t *testing.T) {
	rt := &hookTrustRuntime{}
	s := &Server{runtimeBindings: runtimeBindings{hookTrust: rt}}
	projectRoot := t.TempDir()

	err := s.WorkspaceSetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot,
		Trusted:     true,
	})
	if err != nil {
		t.Fatalf("setTrust: %v", err)
	}
	if rt.calls != 1 || rt.projectRoot != worktree.CanonicalCwd(projectRoot) || !rt.trusted {
		t.Fatalf("trusted root=%q trusted=%v calls=%d, want %q true 1", rt.projectRoot, rt.trusted, rt.calls, worktree.CanonicalCwd(projectRoot))
	}
}

func TestWorkspaceSetHookTrustRejectsUnavailableProjectRoot(t *testing.T) {
	rt := &hookTrustRuntime{}
	s := &Server{runtimeBindings: runtimeBindings{hookTrust: rt}}
	missing := filepath.Join(t.TempDir(), "missing")

	err := s.WorkspaceSetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: missing,
		Trusted:     true,
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("setTrust err = %v, want ErrCwdUnavailable", err)
	}
	if rt.calls != 0 {
		t.Fatalf("trust store calls = %d, want 0", rt.calls)
	}
}
