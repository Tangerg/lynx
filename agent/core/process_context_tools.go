package core

import "context"

// Publish delivers an event to the runtime's listeners, using the current
// action context when called from inside [ProcessContext.ExecuteSafely]. The
// `any`-typed signature avoids a hard dependency on the event package from core.
func (pc *ProcessContext) Publish(event any) {
	pc.PublishContext(pc.eventContext, event)
}

// PublishContext delivers an event to the runtime's listeners with an explicit
// context. Prefer it when publishing from goroutines or helper flows that own a
// more precise context than the current action call.
func (pc *ProcessContext) PublishContext(ctx context.Context, event any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pc.publishContext != nil {
		pc.publishContext(ctx, event)
		return
	}
	if pc.publishEvent != nil {
		pc.publishEvent(event)
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
func (pc *ProcessContext) ResolveTools(ctx context.Context, roles ...string) ([]AgentTool, error) {
	if pc.resolveTools == nil {
		return nil, nil
	}
	return pc.resolveTools(ctx, ToolRolesFor(roles...))
}

// ActionTools resolves the tools declared on the currently-executing
// action's [ActionConfig.ToolGroups]. Returns (nil, nil) when the
// action declared no ToolGroups or no resolver is wired.
func (pc *ProcessContext) ActionTools(ctx context.Context) ([]AgentTool, error) {
	if pc.resolveTools == nil || len(pc.actionToolGroups) == 0 {
		return nil, nil
	}
	return pc.resolveTools(ctx, pc.actionToolGroups)
}

// ToolCallContext derives a child context the runtime can cancel via
// [Process.TerminateToolCall]. The returned cancel func MUST be
// deferred — it both cancels the ctx and detaches the runtime's
// pointer so a later TerminateToolCall doesn't fire on a stale ctx.
// Without a registered canceller, behavior falls back to plain
// [context.WithCancel] (TerminateToolCall becomes a no-op).
func (pc *ProcessContext) ToolCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
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
