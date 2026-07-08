package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Domain types for the publish tests.
type pubTopic struct{ Title string }
type pubBrief struct {
	Topic  string
	Length int
}

// makeBriefingAgent builds a deployable agent that consumes pubTopic,
// produces pubBrief, with a goal whose Export is set so the publish
// helpers pick it up.
func makeBriefingAgent(remote bool) *core.Agent {
	return agent.New("briefing").
		Description("produce a brief from a topic").
		Actions(agent.NewAction("brief",
			func(_ context.Context, _ *core.ProcessContext, t pubTopic) (pubBrief, error) {
				return pubBrief{Topic: t.Title, Length: 100}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[pubBrief](core.Goal{
			Name:        "brief-goal",
			Description: "produce a brief",
			Export: &core.GoalExport{
				Remote:      remote,
				Description: "Produce a one-paragraph topic brief",
				InputSample: pubTopic{},
			},
		})).
		Build()
}

// makeInternalAgent builds an agent whose goal has no Export — the
// publish helpers must skip it.
func makeInternalAgent() *core.Agent {
	return agent.New("internal").
		Actions(agent.NewAction("step",
			func(_ context.Context, _ *core.ProcessContext, t pubTopic) (pubBrief, error) {
				return pubBrief{Topic: t.Title}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[pubBrief](core.Goal{
			Name:        "internal-goal",
			Description: "internal-only",
			// Export deliberately nil — should not appear in either
			// AllAchievableTools or PublishAll.
		})).
		Build()
}

func makeBadExportAgent() *core.Agent {
	return agent.New("bad-export").
		Actions(agent.NewAction("brief",
			func(_ context.Context, _ *core.ProcessContext, t pubTopic) (pubBrief, error) {
				return pubBrief{Topic: t.Title}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[pubBrief](core.Goal{
			Name:        "bad-goal",
			Description: "bad export",
			Export: &core.GoalExport{
				Remote: true,
			},
		})).
		Build()
}

func TestPublishAll_ReturnsTypedSchemaForRemoteGoals(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeBriefingAgent(true), makeInternalAgent())

	tools, err := runtime.PublishAll(platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("PublishAll returned %d tools, want 1 (only briefing has Remote=true Export)", len(tools))
	}
	def := tools[0].Definition()
	if def.Name != "brief-goal" {
		t.Fatalf("Name = %q, want %q", def.Name, "brief-goal")
	}
	if def.Description != "Produce a one-paragraph topic brief" {
		t.Fatalf("Description = %q, want overridden value", def.Description)
	}
	if !strings.Contains(def.InputSchema, "Title") {
		t.Fatalf("InputSchema missing Title field: %s", def.InputSchema)
	}
}

func TestPublishAll_RunsAgentEndToEnd(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeBriefingAgent(true))

	tools, err := runtime.PublishAll(platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("PublishAll returned %d tools, want 1", len(tools))
	}

	args, _ := json.Marshal(pubTopic{Title: "agents"})
	out, err := tools[0].Call(t.Context(), string(args))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var got pubBrief
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.Topic != "agents" {
		t.Fatalf("Topic = %q, want 'agents'", got.Topic)
	}
	if got.Length != 100 {
		t.Fatalf("Length = %d, want 100", got.Length)
	}
}

func TestPublishAll_ExcludesNonRemoteGoals(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform,
		makeBriefingAgent(false), // Remote=false
		makeInternalAgent(),      // no Export
	)

	tools, err := runtime.PublishAll(platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("PublishAll returned %d tools, want 0 (no Remote=true)", len(tools))
	}
}

func TestAllAchievableTools_IncludesAllExportedRegardlessOfRemote(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform,
		makeBriefingAgent(false), // Export.Remote=false but Export!=nil
		makeInternalAgent(),      // Export=nil
	)

	tools, err := runtime.AllAchievableTools(platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("AllAchievableTools returned %d tools, want 1 (only goals with Export!=nil)", len(tools))
	}
	if tools[0].Definition().Name != "brief-goal" {
		t.Fatalf("Name = %q, want brief-goal", tools[0].Definition().Name)
	}
}

func TestAllAchievableTools_RequiresParentInCtx(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeBriefingAgent(true))

	tools, err := runtime.AllAchievableTools(platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool")
	}

	args, _ := json.Marshal(pubTopic{Title: "x"})
	// No parent process in ctx → expect supervisor flow to error.
	_, err = tools[0].Call(t.Context(), string(args))
	if err == nil {
		t.Fatal("expected error: supervisor flow needs parent in ctx")
	}
	if !strings.Contains(err.Error(), "no parent process") {
		t.Fatalf("error = %v, want mention of missing parent", err)
	}
}

func TestPublishAll_NilPlatformReturnsNil(t *testing.T) {
	if got, err := runtime.PublishAll(nil); err != nil || got != nil {
		t.Fatalf("PublishAll(nil) = %v, want nil", got)
	}
	if got, err := runtime.AllAchievableTools(nil); err != nil || got != nil {
		t.Fatalf("AllAchievableTools(nil) = %v, want nil", got)
	}
}

func TestPublishAll_ReturnsSchemaErrorForBadExport(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeBadExportAgent())

	_, err := runtime.PublishAll(platform)
	if err == nil {
		t.Fatal("expected schema error")
	}
	if !strings.Contains(err.Error(), "input sample must not be nil") {
		t.Fatalf("error = %v, want input sample failure", err)
	}
}

func TestGoalExportFor_CapturesInputType(t *testing.T) {
	export := core.GoalExportFor[pubTopic](true)
	if !export.Remote {
		t.Fatal("Remote should be true")
	}
	if _, ok := export.InputSample.(pubTopic); !ok {
		t.Fatalf("InputSample type = %T, want pubTopic", export.InputSample)
	}
}
