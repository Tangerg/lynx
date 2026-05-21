# Lynx Agent

GOAP (Goal-Oriented Action Planning) agent runtime for Go — a port of
[`embabel-agent`](https://github.com/embabel/embabel-agent)'s core ideas
(GOAP + blackboard + OODA loop) shaped to Go idioms.

Implemented and `go test -race ./...` is green. See
[`GUIDE.md`](./GUIDE.md) for the deep guide,
[`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) for the SPI surface,
and [`EMBABEL_COMPARISON.md`](./EMBABEL_COMPARISON.md) for the comparison
against embabel-agent.

## Structure

```
agent/
├── agent.go        fluent agent builder (the recommended user API)
├── builder.go
├── core/           primitives — Action / Goal / Condition / Agent / Blackboard
├── plan/           WorldState, Plan, PlanningSystem, Planner interface
│   └── planner/    goap (A* / default), htn, reactive
├── runtime/        Platform, AgentProcess, sequential/concurrent tick, retry
├── event/          lifecycle event types + multicast listener
├── hitl/           typed Awaitable / Confirmation / Form requests
├── toolpolicy/     OnceOnly / Unlocked chat-tool decorators
├── workflow/       higher-level agent shapes (Loop / Parallel / RepeatUntil / …)
└── examples/       hello (1 action), blog (3-action GOAP plan), supervisor,
                    mcpagent, blogllm, mcpbridge
```

## Quick start

```go
import (
    "context"
    "fmt"

    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
)

type Topic struct{ Title string }
type Post  struct{ Body  string }

func main() {
    a := agent.New("Hello").
        Actions(agent.NewAction("write",
            func(ctx context.Context, pc *core.ProcessContext, t Topic) (Post, error) {
                return Post{Body: "About " + t.Title}, nil
            },
            core.ActionConfig{},
        )).
        Goals(agent.GoalProducing[Post](core.Goal{Description: "post produced"})).
        Build()

    p := agent.NewPlatform(&runtime.PlatformConfig{})
    _ = p.Deploy(a)
    proc, _ := p.RunAgent(context.Background(), a, map[string]any{
        core.DefaultBindingName: Topic{Title: "agents"},
    }, core.ProcessOptions{})
    post, _ := core.ResultOfType[Post](proc)
    fmt.Println(post.Body)
}
```

Run the worked example: `go run ./examples/blog`.
