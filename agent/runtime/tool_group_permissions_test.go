package runtime_test

import (
	"context"
	"errors"
	"slices"
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
func privilegedWebGroup(t *testing.T) core.ToolGroupResolver {
	t.Helper()

	tool, err := tools.New[struct{}, string](
		tools.Config{Name: "web_search"},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	return fixedGroupResolver{
		name: "privileged-web",
		group: fixedToolGroup{info: core.ToolGroupInfo{
			Role:        "web",
			Permissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
		}, tools: []tools.Tool{tool}},
	}
}

type fixedToolGroup struct {
	info  core.ToolGroupInfo
	tools []tools.Tool
}

func (g fixedToolGroup) Info() core.ToolGroupInfo {
	info := g.info
	info.Permissions = slices.Clone(info.Permissions)
	return info
}

func (g fixedToolGroup) Tools(context.Context) ([]tools.Tool, error) {
	return slices.Clone(g.tools), nil
}

type fixedGroupResolver struct {
	name  string
	group core.ToolGroup
}

func (r fixedGroupResolver) Name() string { return r.name }

func (r fixedGroupResolver) Resolve(_ context.Context, requirement core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
	if requirement.Role != r.group.Info().Role {
		return nil, false, nil
	}
	return r.group, true, nil
}

// runActionTools runs a single-action agent whose body calls
// pc.ActionTools with the supplied requirement, and returns what the
// resolver handed back.
func runActionTools(t *testing.T, req core.ToolGroupRequirement) ([]tools.Tool, error) {
	t.Helper()
	return runActionToolsWithResolver(t, req, privilegedWebGroup(t))
}

func runActionToolsWithResolver(t *testing.T, req core.ToolGroupRequirement, resolver core.ToolGroupResolver, additional ...core.Extension) ([]tools.Tool, error) {
	t.Helper()

	var (
		gotTools []tools.Tool
		gotErr   error
	)
	a := agent.New(agent.AgentConfig{Name: "permissions", Actions: []agent.Action{agent.NewAction("probe", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		gotTools, gotErr = pc.ActionTools(ctx)
		return wordCount{Count: len(gotTools)}, nil
	}, core.ActionConfig{ToolGroups: []core.ToolGroupRequirement{req}})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "done"})}})

	extensions := append([]core.Extension{resolver}, additional...)
	engine := agent.MustNewEngine(runtime.Config{Extensions: extensions})
	mustDeploy(t, engine, a)

	if _, err := engine.Run(context.Background(), a,
		core.Input(word{Text: "lynx"}),
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

type panickingToolGroup struct {
	cause   error
	info    core.ToolGroupInfo
	panicIn string
}

func (g panickingToolGroup) Info() core.ToolGroupInfo {
	if g.panicIn == "info" {
		panic(g.cause)
	}
	return g.info
}

func (g panickingToolGroup) Tools(context.Context) ([]tools.Tool, error) {
	panic(g.cause)
}

type panickingToolMiddleware struct{ cause error }

func (panickingToolMiddleware) Name() string { return "panic-tools" }
func (m panickingToolMiddleware) WrapTool(core.ProcessView, core.Action, tools.Tool) tools.Tool {
	panic(m.cause)
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

func TestActionTools_ContainsExtensionPanics(t *testing.T) {
	requirement := core.ToolGroupRequirement{
		Role:               "web",
		AllowedPermissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	}
	t.Run("resolver", func(t *testing.T) {
		cause := errors.New("resolver sentinel")
		_, err := runActionToolsWithResolver(t, requirement, toolGroupResolverFunc(func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
			panic(cause)
		}))
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), `resolver "malformed"`) {
			t.Fatalf("ActionTools error = %v, want attributed resolver panic", err)
		}
	})
	t.Run("middleware", func(t *testing.T) {
		cause := errors.New("middleware sentinel")
		_, err := runActionToolsWithResolver(t, requirement, privilegedWebGroup(t), panickingToolMiddleware{cause: cause})
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), `tool middleware "panic-tools" panicked`) {
			t.Fatalf("ActionTools error = %v, want attributed middleware panic", err)
		}
	})
	for _, method := range []string{"info", "tools"} {
		t.Run("group "+method, func(t *testing.T) {
			cause := errors.New(method + " sentinel")
			resolver := toolGroupResolverFunc(func(context.Context, core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
				return panickingToolGroup{
					cause: cause, panicIn: method,
					info: core.ToolGroupInfo{Role: "web", Permissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess}},
				}, true, nil
			})
			_, err := runActionToolsWithResolver(t, requirement, resolver)
			if !errors.Is(err, cause) || !strings.Contains(strings.ToLower(err.Error()), method) {
				t.Fatalf("ActionTools error = %v, want group %s panic", err, method)
			}
		})
	}
}

func TestActionTools_MalformedResolverReturnsError(t *testing.T) {
	group := func(role string) core.ToolGroup {
		return fixedToolGroup{info: core.ToolGroupInfo{Role: role}}
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
			contains: "role is empty",
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
				return fixedToolGroup{
					info: core.ToolGroupInfo{
						Role:        "web",
						Permissions: []core.ToolGroupPermission{99},
					},
				}, true, nil
			},
			contains: "unknown permission",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := runActionToolsWithResolver(
				t,
				core.ToolGroupRequirement{
					Role: "web",
					AllowedPermissions: []core.ToolGroupPermission{
						core.ToolGroupHostAccess,
						core.ToolGroupInternetAccess,
					},
				},
				test.resolve,
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ActionTools = %v, %v; want error containing %q", resolved, err, test.contains)
			}
		})
	}
}
