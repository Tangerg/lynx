package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// WorkspaceListHooks reports the lifecycle hooks discovered for a cwd — global
// (~/.lyra) + the project's (.lyra) — each marked active iff it currently runs
// (global always; project only when the project is trusted). The client renders
// this for review + a trust toggle (workspace.hooks.list, API.md §7.5).
func (s *Server) WorkspaceListHooks(ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	insp, err := s.workspace.InspectHooks(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("workspace: inspect hooks for %q: %w", root, err)
	}
	out := &protocol.HooksListResult{
		ProjectRoot:    insp.ProjectRoot,
		ProjectTrusted: insp.ProjectTrusted,
		Hooks:          make([]protocol.HookInfo, 0, len(insp.Hooks)),
	}
	for _, h := range insp.Hooks {
		out.Hooks = append(out.Hooks, protocol.HookInfo{
			Event:   string(h.Event),
			Matcher: h.Matcher,
			Command: h.Command,
			Inject:  h.Inject,
			Scope:   string(h.Scope),
			Source:  h.Source,
			Active:  h.Scope == hooks.ScopeGlobal || insp.ProjectTrusted,
		})
	}
	return out, nil
}

// WorkspaceSetHookTrust trusts (or revokes) a project's hooks (workspace.hooks.
// setTrust). The change takes effect on the next turn — the resolver re-reads
// trust per turn.
func (s *Server) WorkspaceSetHookTrust(ctx context.Context, in protocol.SetHookTrustRequest) error {
	if in.ProjectRoot == "" {
		return protocol.ErrInvalidParams
	}
	root, err := workspacepath.ResolveExistingDir(in.ProjectRoot)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", protocol.ErrCwdUnavailable, in.ProjectRoot, err)
	}
	return s.workspace.SetProjectHookTrust(ctx, root, in.Trusted)
}
