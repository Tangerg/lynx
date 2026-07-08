package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Domain types — each action consumes one and produces another.
type (
	Topic    struct{ Title string }
	Outline  struct{ Sections []string }
	Research struct{ Sources []string }
	BlogPost struct {
		Topic    Topic
		Outline  Outline
		Research Research
		Body     string
	}
)

// stubLogger turns every event into one log line — enough to see the OODA
// progression on a real run. Implements [runtime.EventListener] so the
// platform can pick it up via PlatformConfig.Extensions.
type stubLogger struct{}

func (stubLogger) Name() string { return "stub-logger" }
func (stubLogger) OnEvent(_ context.Context, e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.EventName(), e.ProcessID())
}

func main() {
	a := agent.New("BlogAgent").
		Description("synthesize a blog post from a topic").
		Actions(agent.NewAction("research",
			func(ctx context.Context, pc *core.ProcessContext, t Topic) (Research, error) {
				return Research{Sources: []string{"https://example.com/" + t.Title}}, nil
			},
			core.ActionConfig{},
		)).
		Actions(agent.NewAction("outline",
			func(ctx context.Context, pc *core.ProcessContext, t Topic) (Outline, error) {
				return Outline{Sections: []string{"intro", t.Title, "conclusion"}}, nil
			},
			core.ActionConfig{},
		)).
		Actions(agent.NewAction("write",
			// Use Outline as the typed input so the planner can satisfy the
			// generic In via the blackboard. Research is fetched manually
			// from inside the action — the Pre below tells the planner it
			// must also be present before this action becomes applicable.
			func(ctx context.Context, pc *core.ProcessContext, outline Outline) (BlogPost, error) {
				topic, _ := core.Get[Topic](pc.Blackboard, core.DefaultBindingName)
				research, _ := core.Get[Research](pc.Blackboard, core.DefaultBindingName)
				return BlogPost{
					Topic:    topic,
					Outline:  outline,
					Research: research,
					Body:     "Blog about " + topic.Title + " using " + strings.Join(outline.Sections, ", "),
				}, nil
			},
			core.ActionConfig{
				Pre: []string{"it:" + core.TypeName[Research]()},
			},
		)).
		Goals(agent.GoalProducing[BlogPost](core.Goal{Description: "blog post produced"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{stubLogger{}},
	})
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(),
		a,
		map[string]any{core.DefaultBindingName: Topic{Title: "agent-frameworks"}},
		// Switch to ProcessConcurrent to run independent actions in parallel
		// (research + outline on tick 1, write on tick 2).
		core.ProcessOptions{ProcessType: core.ProcessSequential},
	)
	if err != nil {
		log.Fatal(err)
	}

	post, ok := core.ResultOfType[BlogPost](proc)
	if !ok {
		log.Fatalf("no BlogPost produced; status=%s", proc.Status())
	}
	printSummary(proc, post)
}

func printSummary(proc *runtime.AgentProcess, post BlogPost) {
	fmt.Println("\n--- result ---")
	fmt.Printf("status:   %s\n", proc.Status())
	fmt.Printf("topic:    %s\n", post.Topic.Title)
	fmt.Printf("sections: %v\n", post.Outline.Sections)
	fmt.Printf("sources:  %v\n", post.Research.Sources)
	fmt.Printf("body:     %s\n", post.Body)
}
