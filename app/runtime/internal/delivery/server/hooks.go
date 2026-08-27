package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListHooks reports the lifecycle hooks discovered for a cwd — global
// (~/.lyra) + the project's (.lyra) — each marked active iff it currently runs
// (global always; project only when the project is trusted). The client renders
// this for review + a trust toggle (hooks.list, API.md §7.5).
func (s *Server) ListHooks(ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
	insp, err := s.workspaceHooks.Inspect(ctx, in.Workspace.Path)
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
		event, ok := presentHookEvent(h.Event)
		if !ok {
			return nil, fmt.Errorf("hooks.list: unsupported hook event %q", h.Event)
		}
		scope, ok := presentHookScope(h.Scope)
		if !ok {
			return nil, fmt.Errorf("hooks.list: unsupported hook scope %q", h.Scope)
		}
		out.Hooks = append(out.Hooks, protocol.HookInfo{
			Event:         event,
			Matcher:       h.Matcher,
			Command:       h.Command,
			Inject:        h.Inject,
			TimeoutMillis: h.TimeoutMillis,
			Scope:         scope,
			Source:        h.Source,
			Active:        resolved.Active,
		})
	}
	return out, nil
}

func presentHookEvent(event hooks.Event) (protocol.HookEvent, bool) {
	switch event {
	case hooks.PreToolUse:
		return protocol.HookEventPreToolUse, true
	case hooks.PostToolUse:
		return protocol.HookEventPostToolUse, true
	case hooks.UserPromptSubmit:
		return protocol.HookEventUserPromptSubmit, true
	case hooks.SessionStart:
		return protocol.HookEventSessionStart, true
	case hooks.SubagentStart:
		return protocol.HookEventSubagentStart, true
	case hooks.SubagentStop:
		return protocol.HookEventSubagentStop, true
	case hooks.PreCompact:
		return protocol.HookEventPreCompact, true
	case hooks.Stop:
		return protocol.HookEventStop, true
	case hooks.Notification:
		return protocol.HookEventNotification, true
	default:
		return "", false
	}
}

func presentHookScope(scope hooks.Scope) (protocol.HookScope, bool) {
	switch scope {
	case hooks.ScopeGlobal:
		return protocol.HookScopeGlobal, true
	case hooks.ScopeProject:
		return protocol.HookScopeProject, true
	default:
		return "", false
	}
}

// SetHookTrust trusts (or revokes) a project's hooks (hooks.
// setTrust). The change takes effect on the next Run — the resolver re-reads
// trust per Run.
func (s *Server) SetHookTrust(ctx context.Context, in protocol.SetHookTrustRequest) error {
	if in.ProjectRoot == "" {
		return protocol.ErrInvalidParams
	}
	return wireWorkspaceError(s.workspaceHooks.SetProjectTrust(ctx, in.ProjectRoot, in.Trusted))
}
