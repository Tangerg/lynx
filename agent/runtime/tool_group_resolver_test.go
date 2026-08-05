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
	"github.com/Tangerg/lynx/tool"
)

func webGroup(t *testing.T) core.ToolGroupResolver {
	t.Helper()

	return fixedGroupResolver{
		name:  "web",
		role:  "web",
		group: fixedToolGroup{tools: []tool.Tool{newRuntimeTestTool("web_search", nil)}},
	}
}

type fixedToolGroup struct {
	tools []tool.Tool
}

func (g fixedToolGroup) Tools(context.Context) ([]tool.Tool, error) {
	return slices.Clone(g.tools), nil
}

type fixedGroupResolver struct {
	name  string
	role  string
	group core.ToolGroup
}

func (r fixedGroupResolver) Name() string { return r.name }

func (r fixedGroupResolver) Resolve(_ context.Context, role string) (core.ToolGroup, bool, error) {
	if role != r.role {
		return nil, false, nil
	}
	return r.group, true, nil
}

// runActionTools runs a single-action agent whose body calls
// pc.ActionTools with the supplied role, and returns what the
// resolver handed back.
func runActionTools(t *testing.T, role string) ([]tool.Tool, error) {
	t.Helper()
	return runActionToolsWithResolver(t, role, webGroup(t))
}

func runActionToolsWithResolver(t *testing.T, role string, resolver core.ToolGroupResolver, additional ...core.Extension) ([]tool.Tool, error) {
	t.Helper()

	var (
		gotTools []tool.Tool
		gotErr   error
	)
	a := agent.New(agent.Config{Name: "tool-groups", Actions: []agent.Action{agent.NewAction("probe", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		gotTools, gotErr = pc.ActionTools(ctx)
		return wordCount{Count: len(gotTools)}, nil
	}, core.ActionConfig{ToolRoles: []string{role}})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "done"})}})

	extensions := append([]core.Extension{resolver}, additional...)
	engine := agent.MustNewEngine(runtime.Config{Extensions: extensions})
	mustDeploy(t, engine, a)

	if _, err := engine.Run(t.Context(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return gotTools, gotErr
}

type toolGroupResolverFunc func(context.Context, string) (core.ToolGroup, bool, error)

func (toolGroupResolverFunc) Name() string { return "malformed" }
func (f toolGroupResolverFunc) Resolve(ctx context.Context, role string) (core.ToolGroup, bool, error) {
	return f(ctx, role)
}

type panickingToolGroup struct {
	cause error
}

func (g panickingToolGroup) Tools(context.Context) ([]tool.Tool, error) {
	panic(g.cause)
}

type panickingToolMiddleware struct{ cause error }

func (panickingToolMiddleware) Name() string { return "panic-tools" }
func (m panickingToolMiddleware) WrapTool(core.ProcessView, core.ActionDescriptor, tool.Tool) tool.Tool {
	panic(m.cause)
}

func TestActionTools_ResolvesRole(t *testing.T) {
	tools, err := runActionTools(t, "web")
	if err != nil {
		t.Fatalf("ActionTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Definition().Name != "web_search" {
		t.Fatalf("resolved tools = %v, want [web_search]", tools)
	}
}

func TestActionTools_MissingRoleFallsThrough(t *testing.T) {
	tools, err := runActionTools(t, "no-such-role")
	if err != nil {
		t.Fatalf("runActionTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("resolved tools = %v, want none for missing role", tools)
	}
}

func TestActionTools_ContainsExtensionPanics(t *testing.T) {
	role := "web"
	t.Run("resolver", func(t *testing.T) {
		cause := errors.New("resolver sentinel")
		_, err := runActionToolsWithResolver(t, role, toolGroupResolverFunc(func(context.Context, string) (core.ToolGroup, bool, error) {
			panic(cause)
		}))
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), `resolver "malformed"`) {
			t.Fatalf("ActionTools error = %v, want attributed resolver panic", err)
		}
	})
	t.Run("middleware", func(t *testing.T) {
		cause := errors.New("middleware sentinel")
		_, err := runActionToolsWithResolver(t, role, webGroup(t), panickingToolMiddleware{cause: cause})
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), `tool middleware "panic-tools" panicked`) {
			t.Fatalf("ActionTools error = %v, want attributed middleware panic", err)
		}
	})
	t.Run("group tools", func(t *testing.T) {
		cause := errors.New("tools sentinel")
		resolver := toolGroupResolverFunc(func(context.Context, string) (core.ToolGroup, bool, error) {
			return panickingToolGroup{cause: cause}, true, nil
		})
		_, err := runActionToolsWithResolver(t, role, resolver)
		if !errors.Is(err, cause) || !strings.Contains(strings.ToLower(err.Error()), "tools") {
			t.Fatalf("ActionTools error = %v, want group tools panic", err)
		}
	})
}

func TestActionTools_MalformedResolverReturnsError(t *testing.T) {
	group := fixedToolGroup{}
	tests := []struct {
		name     string
		resolve  toolGroupResolverFunc
		contains string
	}{
		{
			name: "group on miss",
			resolve: func(context.Context, string) (core.ToolGroup, bool, error) {
				return group, false, nil
			},
			contains: "group for a miss",
		},
		{
			name: "nil matched group",
			resolve: func(context.Context, string) (core.ToolGroup, bool, error) {
				return nil, true, nil
			},
			contains: "nil group",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := runActionToolsWithResolver(
				t,
				"web",
				test.resolve,
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ActionTools = %v, %v; want error containing %q", resolved, err, test.contains)
			}
		})
	}
}
