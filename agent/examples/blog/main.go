// Blog is a richer example that exercises the planner: three actions
// (research, outline, write) chained by data dependencies, one terminal goal.
// No LLM is needed — each action returns a stub artifact — but the GOAP
// planner has to figure out the topological order on its own.
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
// progression on a real run.
type stubLogger struct{}

func (stubLogger) OnEvent(e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.EventName(), e.ProcessID())
}

func main() {
	a := agent.New("BlogAgent").
		Description("synthesize a blog post from a topic").
		Actions(agent.NewAction("research",
			func(ctx context.Context, pc *core.ProcessContext, t Topic) (Research, error) {
				return Research{Sources: []string{"https://example.com/" + t.Title}}, nil
			},
		)).
		Actions(agent.NewAction("outline",
			func(ctx context.Context, pc *core.ProcessContext, t Topic) (Outline, error) {
				return Outline{Sections: []string{"intro", t.Title, "conclusion"}}, nil
			},
		)).
		Actions(agent.NewAction("write",
			// Use Outline as the typed input so the planner can satisfy the
			// generic In via the blackboard. Research is fetched manually
			// from inside the action — the WithPre below tells the planner
			// it must also be present before this action becomes
			// applicable.
			func(ctx context.Context, pc *core.ProcessContext, outline Outline) (BlogPost, error) {
				topic, _ := core.Get[Topic](pc.Blackboard, core.DefaultBinding)
				research, _ := core.Get[Research](pc.Blackboard, core.DefaultBinding)
				return BlogPost{
					Topic:    topic,
					Outline:  outline,
					Research: research,
					Body:     "Blog about " + topic.Title + " using " + strings.Join(outline.Sections, ", "),
				}, nil
			},
			core.WithPre("it:"+core.TypeFullNameOf[Research]()),
		)).
		Goals(agent.GoalProducing[BlogPost]("blog post produced")).
		Build()

	platform := agent.NewPlatform(
		agent.WithListener(event.ListenerFunc(stubLogger{}.OnEvent)),
	)
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(),
		a,
		map[string]any{core.DefaultBinding: Topic{Title: "agent-frameworks"}},
		// Switch to ProcessConcurrent to run independent actions in parallel
		// (research + outline on tick 1, write on tick 2).
		core.WithProcessType(core.ProcessSimple),
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
