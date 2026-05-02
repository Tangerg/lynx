# Lynx Agent

GOAP (Goal-Oriented Action Planning) agent runtime for Go — a port of
[`embabel-agent`](https://github.com/embabel/embabel-agent)'s core ideas
(GOAP + blackboard + OODA loop) shaped to Go idioms.

Status: M0–M5 of [`doc/agent/06-rollout.md`](../doc/agent/06-rollout.md)
implemented. `go test -race ./...` is green.

## Structure

```
agent/
├── core/          primitives — Action / Goal / Condition / Agent / Blackboard
├── plan/          WorldState, Plan, PlanningSystem, Planner interface
├── planner/goap/  A* GOAP planner
├── runtime/       Platform, AgentProcess, simple/concurrent tick, retry loop
├── event/         lifecycle event types + multicast listener
├── dsl/           fluent agent builder (the recommended user API)
├── hitl/          typed Awaitable / Confirmation / Form requests
├── reflect/       struct-method-driven agent registration (alternative)
└── examples/      hello (1 action), blog (3-action GOAP plan)
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
    a := agent.New(core.AgentMeta{Name: "Hello"}).
        Actions(agent.NewAction("write",
            func(ctx context.Context, pc *core.ProcessContext, t Topic) (Post, error) {
                return Post{Body: "About " + t.Title}, nil
            },
            core.ActionConfig{},
        )).
        Goals(agent.GoalProducing[Post](core.Goal{Description: "post produced"})).
        Build()

    p := agent.NewPlatform(runtime.PlatformConfig{})
    _ = p.Deploy(a)
    proc, _ := p.RunAgent(context.Background(), a, map[string]any{
        core.DefaultBinding: Topic{Title: "agents"},
    }, core.ProcessOptions{})
    post, _ := core.ResultOfType[Post](proc)
    fmt.Println(post.Body)
}
```

Run the worked example: `go run ./examples/blog`.
