package runtime

import "github.com/Tangerg/lynx/agent/core"

// buildProcessContext assembles a fresh ProcessContext for one tick. The
// fields all live on AgentProcess; the context is re-created every tick so
// per-action state (lastErr, etc.) doesn't leak. actionToolGroups is the
// currently-executing action's declared requirements; threading it in lets
// [core.ProcessContext.ActionTools] resolve them lazily.
func (p *AgentProcess) buildProcessContext(actionToolGroups []core.ToolGroupRequirement, action core.Action) *core.ProcessContext {
	config := core.ProcessContextConfig{
		ProcessState: core.ProcessState{
			Process:       p,
			Blackboard:    p.blackboard,
			Options:       p.options,
			OutputChannel: p.options.OutputChannel,
			Services:      p.platformServices(),
		},
		PlatformHooks: core.PlatformHooks{
			ChatClient:     p.effectiveChatClient(),
			Guardrails:     p.effectiveGuardrails(),
			Publish:        p.publishAny,
			ResolveTools:   p.toolResolverFor(action),
			ToolCallCancel: p.signals.registerToolCallCancel,
		},
		ActionToolGroups: actionToolGroups,
	}
	return core.NewProcessContext(config)
}
