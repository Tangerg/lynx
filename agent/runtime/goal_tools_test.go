package runtime_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type briefTopic struct{ Title string }
type briefResult struct {
	Topic  string
	Length int
}

// makeBriefingAgent builds a deployable agent that consumes briefTopic,
// produces briefResult, with a configured goal tool so the runtime
// helpers pick it up.
func makeBriefingAgent(standalone bool) *core.Agent {
	return agent.New(agent.AgentConfig{Name: "briefing", Description: "produce a brief from a topic", Actions: []agent.Action{agent.NewAction("brief", func(_ context.Context, _ *core.ProcessContext, topic briefTopic) (briefResult, error) {
		return briefResult{Topic: topic.Title, Length: 100}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[briefResult](core.GoalConfig{Name: "brief-goal", Description: "produce a brief", Tool: core.NewGoalTool[briefTopic](core.GoalToolConfig{Standalone: standalone, Description: "Produce a one-paragraph topic brief"})})}})
}

// makeInternalAgent builds an agent whose goal is not exposed as a tool.
func makeInternalAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "internal", Actions: []agent.Action{agent.NewAction("step", func(_ context.Context, _ *core.ProcessContext, topic briefTopic) (briefResult, error) {
		return briefResult{Topic: topic.Title}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[briefResult](core.GoalConfig{Name: "internal-goal", Description: "internal-only"})}})
}

func makeInvalidGoalToolAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "invalid-tool", Actions: []agent.Action{agent.NewAction("brief", func(_ context.Context, _ *core.ProcessContext, topic briefTopic) (briefResult, error) {
		return briefResult{Topic: topic.Title}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[briefResult](core.GoalConfig{Name: "invalid-goal", Description: "invalid tool input", Tool: core.NewGoalTool[any](core.GoalToolConfig{Standalone: true})})}})
}

func TestStandaloneGoalTools_ReturnTypedSchema(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeBriefingAgent(true), makeInternalAgent())

	tools, err := runtime.StandaloneGoalTools(engine)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("StandaloneGoalTools returned %d tools, want 1", len(tools))
	}
	definition := tools[0].Definition()
	if definition.Name != "brief-goal" {
		t.Fatalf("Name = %q, want %q", definition.Name, "brief-goal")
	}
	if definition.Description != "Produce a one-paragraph topic brief" {
		t.Fatalf("Description = %q, want overridden value", definition.Description)
	}
	if !strings.Contains(string(definition.InputSchema), "Title") {
		t.Fatalf("InputSchema missing Title field: %s", definition.InputSchema)
	}
}

func TestStandaloneGoalTools_RunAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeBriefingAgent(true))

	tools, err := runtime.StandaloneGoalTools(engine)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("StandaloneGoalTools returned %d tools, want 1", len(tools))
	}

	arguments, _ := json.Marshal(briefTopic{Title: "agents"})
	output, err := tools[0].Call(t.Context(), string(arguments))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var got briefResult
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.Topic != "agents" {
		t.Fatalf("Topic = %q, want 'agents'", got.Topic)
	}
	if got.Length != 100 {
		t.Fatalf("Length = %d, want 100", got.Length)
	}
}

func TestStandaloneGoalTools_ExcludeChildOnlyGoals(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine,
		makeBriefingAgent(false), // Standalone=false
		makeInternalAgent(),      // no goal tool
	)

	tools, err := runtime.StandaloneGoalTools(engine)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("StandaloneGoalTools returned %d tools, want 0 (no Standalone=true)", len(tools))
	}
}

func TestGoalTools_IncludeChildGoalTools(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine,
		makeBriefingAgent(false), // child-only goal tool
		makeInternalAgent(),      // no goal tool
	)

	tools, err := runtime.GoalTools(engine)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("GoalTools returned %d tools, want 1", len(tools))
	}
	if tools[0].Definition().Name != "brief-goal" {
		t.Fatalf("Name = %q, want brief-goal", tools[0].Definition().Name)
	}
}

func TestGoalTools_RequireParentProcess(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeBriefingAgent(true))

	tools, err := runtime.GoalTools(engine)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool")
	}

	arguments, _ := json.Marshal(briefTopic{Title: "x"})
	// No parent process in ctx → expect supervisor flow to error.
	_, err = tools[0].Call(t.Context(), string(arguments))
	if err == nil {
		t.Fatal("expected error: supervisor flow needs parent in ctx")
	}
	if !strings.Contains(err.Error(), "no parent process") {
		t.Fatalf("error = %v, want mention of missing parent", err)
	}
}

func TestGoalTools_RejectNilEngine(t *testing.T) {
	if _, err := runtime.StandaloneGoalTools(nil); err == nil {
		t.Fatal("StandaloneGoalTools(nil) returned nil error")
	}
	if _, err := runtime.GoalTools(nil); err == nil {
		t.Fatal("GoalTools(nil) returned nil error")
	}
}

func TestDeployRejectsGoalToolWithInterfaceInput(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.Deploy(makeInvalidGoalToolAgent())
	if err == nil {
		t.Fatal("expected schema error")
	}
	if !strings.Contains(err.Error(), "input type must not be an interface") {
		t.Fatalf("error = %v, want input type failure", err)
	}
}

func TestNewGoalToolCapturesInputType(t *testing.T) {
	goalTool := core.NewGoalTool[briefTopic](core.GoalToolConfig{Standalone: true})
	if !goalTool.Standalone {
		t.Fatal("Standalone should be true")
	}
	if got := goalTool.InputType(); got != reflect.TypeFor[briefTopic]() {
		t.Fatalf("InputType = %v, want %v", got, reflect.TypeFor[briefTopic]())
	}
}
