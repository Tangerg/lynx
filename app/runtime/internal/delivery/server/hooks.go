package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListHooks reports the lifecycle hooks discovered for a cwd — global
// (~/.lyra) + the project's (.lyra) — each marked active iff it currently runs
// (global always; project only when the project is trusted). The client renders
// this for review + a trust toggle (hooks.list, API.md §7.5).
func (s *Server) ListHooks(ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
	insp, err := s.workspaceHooks.InspectHooks(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(fmt.Errorf("workspace: inspect hooks: %w", err))
	}
	out := &protocol.HooksListResult{
		ProjectRoot:    insp.ProjectRoot,
		ProjectTrusted: insp.ProjectTrusted,
		Hooks:          make([]protocol.HookInfo, 0, len(insp.Hooks)),
	}
	for _, resolved := range insp.Hooks {
		h := resolved.Hook
		out.Hooks = append(out.Hooks, protocol.HookInfo{
			Event:     protocol.HookEvent(h.Event),
			Matcher:   h.Matcher,
			Command:   h.Command,
			Inject:    h.Inject,
			TimeoutMs: h.TimeoutMs,
			Scope:     string(h.Scope),
			Source:    h.Source,
			Active:    resolved.Active,
		})
	}
	return out, nil
}

// SetHookTrust trusts (or revokes) a project's hooks (hooks.
// setTrust). The change takes effect on the next turn — the resolver re-reads
// trust per turn.
func (s *Server) SetHookTrust(ctx context.Context, in protocol.SetHookTrustRequest) error {
	if in.ProjectRoot == "" {
		return protocol.ErrInvalidParams
	}
	return wireWorkspaceError(s.workspaceHooks.SetProjectHookTrust(ctx, in.ProjectRoot, in.Trusted))
}
