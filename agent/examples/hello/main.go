package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
)

// CountResult is what the agent ultimately produces — the goal references it
// by type so the planner knows when we're done.
type CountResult struct {
	Length int
}

func main() {
	a := agent.New(agent.AgentConfig{Name: "Hello", Description: "count uppercase characters of a phrase", Actions: []agent.Action{agent.NewAction("count_upper", func(ctx context.Context, pc *agent.ProcessContext, in string) (CountResult, error) {
		upper := strings.ToUpper(in)
		return CountResult{Length: len(upper)}, nil
	}, agent.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[CountResult](agent.GoalConfig{Description: "uppercase length determined"})}})

	engine := agent.MustNewEngine(agent.EngineConfig{})
	if _, err := engine.Deploy(a); err != nil {
		log.Fatal(err)
	}

	process, err := engine.Run(
		context.Background(),
		a,
		map[string]any{agent.DefaultBindingName: "hello"},
		agent.ProcessOptions{},
	)
	if err != nil {
		log.Fatalf("Run failed: %v", err)
	}

	result, ok := agent.Result[CountResult](process)
	if !ok {
		log.Fatalf("agent produced no CountResult; status=%s", process.Status())
	}
	fmt.Printf("status=%s length=%d\n", process.Status(), result.Length)
}
