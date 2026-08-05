package delegation

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Input is the complete model-facing contract for one delegated task. Summary
// identifies the child in lifecycle projections; Instructions are the child's
// isolated input.
type Input struct {
	Summary      string `json:"summary" jsonschema:"minLength=1,maxLength=80,pattern=^[^[:space:]](.*[^[:space:]])?$" jsonschema_description:"Concise 3-5 word action label, at most 80 characters, that identifies this delegated task. Do not include leading or trailing whitespace."`
	Instructions string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained work instructions. The delegated Agent cannot see the parent conversation, so include every fact it needs."`
}

// Execute runs the isolated instructions inside the child process and returns
// the final answer exposed to the parent tool call.
type Execute func(context.Context, *core.ProcessContext, string) (string, error)

// NewAgent defines the Agent deployed behind the delegation tool. The toolset
// owns its model contract and role visibility; the execution engine supplies
// only the callback that performs one child interaction.
func NewAgent(execute Execute) *core.Agent {
	action := agent.NewAction(
		"delegated_task",
		func(ctx context.Context, process *core.ProcessContext, input Input) (string, error) {
			return execute(ctx, process, input.Instructions)
		},
		core.ActionConfig{ToolRoles: []string{tool.GroupDelegated}},
	)
	answer := agent.NewOutputGoal[string](
		core.GoalConfig{Description: "delegated task answer produced"},
	)
	return agent.New(agent.Config{
		Name: catalog.DelegateTask,
		Description: "Delegate one self-contained task to a fresh Agent with coding tools and bounded delegation. " +
			"Use it for focused, separable work so the current context stays uncluttered. " +
			"The delegated Agent starts with clean context and cannot see its parent conversation, so include everything it needs in instructions. " +
			"It returns one final answer.",
		Actions: []agent.Action{action},
		Goals:   []*agent.Goal{answer},
	})
}
