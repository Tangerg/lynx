package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
)

// privilegedWebGroup returns a resolver pre-loaded with a tool group
// that grants internet access — the shape a sandboxed deployment must
// reject unless the action explicitly opts in.
func privilegedWebGroup(t *testing.T) *core.StaticToolGroupResolver {
	t.Helper()

	tool, err := chat.NewTool[struct{}, string](
		chat.ToolDefinition{Name: "web_search"},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	resolver := core.NewStaticToolGroupResolver("privileged-web")
	resolver.Register("web", core.NewLazyToolGroup(
		core.SimpleToolGroupMetadata{
			RoleText:           "web",
			PermissionsGranted: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
		},
		func(context.Context) ([]core.AgentTool, error) { return []core.AgentTool{tool}, nil },
	))
	return resolver
}

// runActionTools runs a single-action agent whose body calls
// pc.ActionTools with the supplied requirement, and returns what the
// resolver handed back.
func runActionTools(t *testing.T, req core.ToolGroupRequirement) ([]core.AgentTool, error) {
	t.Helper()

	var (
		gotTools []core.AgentTool
		gotErr   error
	)
	a := agent.New("permissions").
		Actions(agent.NewAction("probe",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				gotTools, gotErr = pc.ActionTools(ctx)
				return wordCount{Count: len(gotTools)}, nil
			},
			core.ActionConfig{ToolGroups: []core.ToolGroupRequirement{req}},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "done"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{privilegedWebGroup(t)},
	})
	mustDeploy(t, platform, a)

	if _, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{}); err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	return gotTools, gotErr
}

// TestActionTools_PermissionOptInFlowsToResolver is the regression
// test for the role-string funnel that used to drop the action's
// declared Permissions: a requirement that opts into the group's
// privileges must resolve the tools.
func TestActionTools_PermissionOptInFlowsToResolver(t *testing.T) {
	tools, err := runActionTools(t, core.ToolGroupRequirement{
		Role:        "web",
		Permissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	})
	if err != nil {
		t.Fatalf("ActionTools with opt-in: %v", err)
	}
	if len(tools) != 1 || tools[0].Definition().Name != "web_search" {
		t.Fatalf("resolved tools = %v, want [web_search]", tools)
	}
}

// TestActionTools_PrivilegedGroupRejectedWithoutOptIn — an empty
// Permissions slice means "no special privileges": the high-privilege
// group must be rejected at the dispatch site, not silently granted.
func TestActionTools_PrivilegedGroupRejectedWithoutOptIn(t *testing.T) {
	tools, err := runActionTools(t, core.ToolGroupRequirement{Role: "web"})
	if err == nil {
		t.Fatalf("expected permission rejection, got tools %v", tools)
	}
	if !strings.Contains(err.Error(), "exceeding requirement") {
		t.Fatalf("error = %v, want permission-exceeded rejection", err)
	}
}

func TestActionTools_MissingRoleFallsThrough(t *testing.T) {
	tools, err := runActionTools(t, core.ToolGroupRequirement{Role: "no-such-role"})
	if err != nil {
		t.Fatalf("runActionTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("resolved tools = %v, want none for missing role", tools)
	}
}
