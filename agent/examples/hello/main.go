package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
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
			func(ctx context.Context, pc *core.ProcessContext, in string) (CountResult, error) {
				upper := strings.ToUpper(in)
				return CountResult{Length: len(upper)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[CountResult](core.Goal{Description: "uppercase length determined"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(),
		a,
		map[string]any{core.DefaultBindingName: "hello"},
		core.ProcessOptions{},
	)
	if err != nil {
		log.Fatalf("RunAgent failed: %v", err)
	}

	result, ok := core.ResultOfType[CountResult](proc)
	if !ok {
		log.Fatalf("agent produced no CountResult; status=%s", proc.Status())
	}
	fmt.Printf("status=%s length=%d\n", proc.Status(), result.Length)
}
