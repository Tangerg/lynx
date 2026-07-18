package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

func TestSubtaskRoleCanAskAndExitPlanWithoutDelegating(t *testing.T) {
	policy, err := approval.New(approval.ModeBalanced, nil)
	if err != nil {
		t.Fatalf("approval policy: %v", err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Workdir:  t.TempDir(),
		Approval: policy,
		Interrupt: func(context.Context, string, any) (interrupts.Resolution, error) {
			return interrupts.Resolution{}, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		for _, close := range built.Closers {
			_ = close()
		}
	})

	group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleSubtask})
	if err != nil || !ok {
		t.Fatalf("Resolve(subtask) = %v, %v", ok, err)
	}
	tools, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Definition().Name] = true
	}
	if !names["ask_user"] || !names["exit_plan_mode"] {
		t.Fatalf("subtask tools = %v, want ask_user and exit_plan_mode", names)
	}
	if names["task"] || names["schedule"] {
		t.Fatalf("subtask tools = %v, task/schedule must stay root-only", names)
	}
}
