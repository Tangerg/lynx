package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
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
// progression on a real run. It implements the event-listener extension so
// the engine can pick it up via Config.Extensions.
type stubLogger struct{}

func (stubLogger) Name() string { return "stub-logger" }
func (stubLogger) OnEvent(_ context.Context, e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.Kind(), e.ProcessID())
}

func main() {
	a := agent.New(agent.AgentConfig{Name: "BlogAgent", Description: "synthesize a blog post from a topic", Actions: []agent.Action{agent.NewAction("research", func(ctx context.Context, pc *agent.ProcessContext, t Topic) (Research, error) {
		return Research{Sources: []string{"https://example.com/" + t.Title}}, nil
	}, agent.ActionConfig{}), agent.NewAction("outline", func(ctx context.Context, pc *agent.ProcessContext, t Topic) (Outline, error) {
		return Outline{Sections: []string{"intro", t.Title, "conclusion"}}, nil
	}, agent.ActionConfig{}), agent.NewAction("write", func(ctx context.Context, pc *agent.ProcessContext, outline Outline) (BlogPost, error) {
		topic, _ := core.Get[Topic](pc.Blackboard(), core.DefaultBindingName)
		research, _ := core.Get[Research](pc.Blackboard(), core.DefaultBindingName)
		return BlogPost{Topic: topic, Outline: outline, Research: research, Body: "Blog about " + topic.Title + " using " + strings.Join(outline.Sections, ", ")}, nil
	}, agent.ActionConfig{Preconditions: []string{"it:" + core.TypeName[Research]()}})}, Goals: []*agent.Goal{agent.NewOutputGoal[BlogPost](agent.GoalConfig{Description: "blog post produced"})}})

	engine := agent.MustNewEngine(agent.EngineConfig{
		Extensions: []agent.Extension{stubLogger{}},
	})
	if _, err := engine.Deploy(context.Background(), a); err != nil {
		log.Fatal(err)
	}

	process, err := engine.Run(
		context.Background(),
		a,
		agent.Input(Topic{Title: "agent-frameworks"}),
		agent.ProcessOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	post, ok := agent.Result[BlogPost](process)
	if !ok {
		log.Fatalf("no BlogPost produced; status=%s", process.Status())
	}
	printSummary(process, post)
}

func printSummary(process *agent.Process, post BlogPost) {
	fmt.Println("\n--- result ---")
	fmt.Printf("status:   %s\n", process.Status())
	fmt.Printf("topic:    %s\n", post.Topic.Title)
	fmt.Printf("sections: %v\n", post.Outline.Sections)
	fmt.Printf("sources:  %v\n", post.Research.Sources)
	fmt.Printf("body:     %s\n", post.Body)
}
