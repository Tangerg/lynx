package core

import (
	"context"

	"github.com/Tangerg/lynx/tools"
)

// Publish delivers an event to the runtime's listeners. The `any`-typed
// signature avoids a hard dependency on the event package from core.
func (pc *ProcessContext) Publish(ctx context.Context, event any) {
	if pc.publishEvent != nil {
		pc.publishEvent(contextOrBackground(ctx), event)
	}
}

// ResolveTools turns a list of role names into concrete tools via the
// platform-configured resolver. Returns (nil, nil) when no resolver
// is wired or no roles are supplied; the caller decides whether
// missing tools are fatal.
//
// Each role resolves with empty [ToolGroupRequirement.Permissions] —
// "no special privileges" — so high-privilege tool groups are rejected
// at the dispatch site. Actions that need such groups declare them via
// [ActionConfig.ToolGroups] with explicit permissions and use
// [ProcessContext.ActionTools] instead.
func (pc *ProcessContext) ResolveTools(ctx context.Context, roles ...string) ([]tools.Tool, error) {
	if pc.resolveTools == nil {
		return nil, nil
	}
	return pc.resolveTools(contextOrBackground(ctx), ToolRolesFor(roles...))
}

// ActionTools resolves the tools declared on the currently-executing
// action's [ActionConfig.ToolGroups]. Returns (nil, nil) when the
// action declared no ToolGroups or no resolver is wired.
func (pc *ProcessContext) ActionTools(ctx context.Context) ([]tools.Tool, error) {
	if pc.resolveTools == nil || len(pc.actionToolGroups) == 0 {
		return nil, nil
	}
	return pc.resolveTools(contextOrBackground(ctx), pc.actionToolGroups)
}

// ToolCallContext derives a child context the runtime can cancel via
// [Process.TerminateToolCall]. The returned cancel func MUST be
// deferred — it both cancels the ctx and detaches the runtime's
// pointer so a later TerminateToolCall doesn't fire on a stale ctx.
// Without a registered canceller, behavior falls back to plain
// [context.WithCancel] (TerminateToolCall becomes a no-op).
func (pc *ProcessContext) ToolCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(contextOrBackground(parent))
	if pc.toolCallCancel == nil {
		return ctx, cancel
	}
	release := pc.toolCallCancel(cancel)
	return ctx, func() {
		cancel()
		if release != nil {
			release()
		}
	}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
