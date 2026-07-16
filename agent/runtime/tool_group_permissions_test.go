package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/tools"
)

// privilegedWebGroup returns a resolver pre-loaded with a tool group
// that grants internet access — the shape a sandboxed deployment must
// reject unless the action explicitly opts in.
func privilegedWebGroup(t *testing.T) *core.StaticToolGroupResolver {
	t.Helper()

	tool, err := tools.New[struct{}, string](
		tools.Config{Name: "web_search"},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	resolver := core.NewStaticToolGroupResolver("privileged-web")
	resolver.Set("web", core.NewLazyToolGroup(
		core.ToolGroupInfo{
			Role:        "web",
			Permissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
		},
		func(context.Context) ([]tools.Tool, error) { return []tools.Tool{tool}, nil },
	))
	return resolver
}

// runActionTools runs a single-action agent whose body calls
// pc.ActionTools with the supplied requirement, and returns what the
// resolver handed back.
func runActionTools(t *testing.T, req core.ToolGroupRequirement) ([]tools.Tool, error) {
	t.Helper()
	return runActionToolsWithResolver(t, req, privilegedWebGroup(t))
}

func runActionToolsWithResolver(t *testing.T, req core.ToolGroupRequirement, resolver core.ToolGroupResolver) ([]tools.Tool, error) {
	t.Helper()

	var (
		gotTools []tools.Tool
		gotErr   error
	)
	a := agent.New(agent.AgentConfig{Name: "permissions", Actions: []agent.Action{agent.NewAction("probe", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		gotTools, gotErr = pc.ActionTools(ctx)
		return wordCount{Count: len(gotTools)}, nil
	}, core.ActionConfig{ToolGroups: []core.ToolGroupRequirement{req}})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{resolver},
	})
	mustDeploy(t, engine, a)

	if _, err := engine.Run(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return gotTools, gotErr
}

type toolGroupResolverFunc func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error)

func (toolGroupResolverFunc) Name() string { return "malformed" }
func (f toolGroupResolverFunc) Resolve(ctx context.Context, requirement core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
	return f(ctx, requirement)
}

type emptyInfoToolGroup struct{}

func (emptyInfoToolGroup) Info() core.ToolGroupInfo { return core.ToolGroupInfo{} }
func (emptyInfoToolGroup) Tools(context.Context) ([]tools.Tool, error) {
	return nil, nil
}

// TestActionTools_PermissionOptInFlowsToResolver is the regression
// test for the role-string funnel that used to drop the action's
// declared AllowedPermissions: a requirement that opts into the group's
// privileges must resolve the tools.
func TestActionTools_PermissionOptInFlowsToResolver(t *testing.T) {
	tools, err := runActionTools(t, core.ToolGroupRequirement{
		Role:               "web",
		AllowedPermissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	})
	if err != nil {
		t.Fatalf("ActionTools with opt-in: %v", err)
	}
	if len(tools) != 1 || tools[0].Definition().Name != "web_search" {
		t.Fatalf("resolved tools = %v, want [web_search]", tools)
	}
}

// TestActionTools_PrivilegedGroupRejectedWithoutOptIn — an empty
// AllowedPermissions slice means "no special privileges": the high-privilege
// group must be rejected at the dispatch site, not silently granted.
func TestActionTools_PrivilegedGroupRejectedWithoutOptIn(t *testing.T) {
	tools, err := runActionTools(t, core.ToolGroupRequirement{Role: "web"})
	if err == nil {
		t.Fatalf("expected permission rejection, got tools %v", tools)
	}
	if !strings.Contains(err.Error(), "requires permissions") {
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

func TestActionTools_MalformedResolverReturnsError(t *testing.T) {
	group := func(role string) core.ToolGroup {
		return core.NewLazyToolGroup(
			core.ToolGroupInfo{Role: role},
			func(context.Context) ([]tools.Tool, error) { return nil, nil },
		)
	}
	tests := []struct {
		name     string
		resolve  toolGroupResolverFunc
		contains string
	}{
		{
			name: "group on miss",
			resolve: func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return group("web"), false, nil
			},
			contains: "group for a miss",
		},
		{
			name: "nil matched group",
			resolve: func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return nil, true, nil
			},
			contains: "nil group",
		},
		{
			name: "empty group role",
			resolve: func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return emptyInfoToolGroup{}, true, nil
			},
			contains: "empty group role",
		},
		{
			name: "wrong group role",
			resolve: func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return group("other"), true, nil
			},
			contains: "group role",
		},
		{
			name: "unknown permission",
			resolve: func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return core.NewLazyToolGroup(
					core.ToolGroupInfo{
						Role:        "web",
						Permissions: []core.ToolGroupPermission{99},
					},
					func(context.Context) ([]tools.Tool, error) { return nil, nil },
				), true, nil
			},
			contains: "unknown permission",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := runActionToolsWithResolver(
				t,
				core.ToolGroupRequirement{Role: "web", AllowedPermissions: []core.ToolGroupPermission{99}},
				test.resolve,
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ActionTools = %v, %v; want error containing %q", resolved, err, test.contains)
			}
		})
	}
}
