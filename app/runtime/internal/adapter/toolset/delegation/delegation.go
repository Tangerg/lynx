package delegation

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

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
		Name:        catalog.DelegateTask,
		Description: Description,
		Actions:     []agent.Action{action},
		Goals:       []*agent.Goal{answer},
	})
}
