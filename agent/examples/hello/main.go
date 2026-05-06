// Hello demonstrates the smallest end-to-end agent: take a string in, produce
// its uppercase length, and report success. No LLM calls, no tools — just
// the OODA loop, the GOAP planner, and the blackboard wiring.
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
	a := agent.New("Hello").
		Description("count uppercase characters of a phrase").
		Actions(agent.NewAction("count_upper",
			func(ctx context.Context, pc *agent.ProcessContext, in string) (CountResult, error) {
				upper := strings.ToUpper(in)
				return CountResult{Length: len(upper)}, nil
			},
			agent.ActionConfig{},
		)).
		Goals(agent.GoalProducing[CountResult](agent.Goal{Description: "uppercase length determined"})).
		Build()

	platform := agent.NewPlatform(agent.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(),
		a,
		map[string]any{agent.DefaultBinding: "hello"},
		agent.ProcessOptions{},
	)
	if err != nil {
		log.Fatalf("RunAgent failed: %v", err)
	}

	result, ok := agent.ResultOfType[CountResult](proc)
	if !ok {
		log.Fatalf("agent produced no CountResult; status=%s", proc.Status())
	}
	fmt.Printf("status=%s length=%d\n", proc.Status(), result.Length)
}
