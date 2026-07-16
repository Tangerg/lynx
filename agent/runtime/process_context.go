package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

// buildProcessContext assembles a fresh ProcessContext for one tick. The
// fields all live on Process; the context is re-created every tick so
// per-action suspension state doesn't leak. actionToolGroups is the currently
// executing action's declared requirements; threading it in lets
// [core.ProcessContext.ActionTools] resolve them lazily.
func (p *Process) buildProcessContext(actionToolGroups []core.ToolGroupRequirement, action core.Action) *core.ProcessContext {
	maxToolRounds := 0
	if guardrails := p.effectiveGuardrails(); guardrails != nil {
		maxToolRounds = guardrails.MaxToolRounds
	}
	config := core.ProcessContextConfig{
		Process:       p,
		Control:       processControl{process: p},
		Usage:         processUsage{process: p},
		Blackboard:    p.blackboard,
		Session:       p.options.Session,
		Dependencies:  p.dependencies.Child(),
		Chat:          p.effectiveChat,
		MaxToolRounds: maxToolRounds,
		Emit:          p.publishAny,
		ResolveTools:  p.toolResolverFor(action),
		RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return p.runInteraction(ctx, action.Metadata().Name, input)
		},
		ToolCallCancel:   p.signals.registerToolCallCancel,
		ActionToolGroups: actionToolGroups,
	}
	return core.NewProcessContext(config)
}
