package workflow_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

// Domain types for the sequence test: each agent takes the previous
// agent's output and produces a richer type.
type seqTopic struct{ Word string }
type seqOutline struct{ Sections []string }
type seqDraft struct{ Body string }

// makeOutlineAgent: takes seqTopic, produces seqOutline.
func makeOutlineAgent() *core.Agent {
	a := agent.New("outline-agent").
		Description("expand a topic into an outline").
		Actions(agent.NewAction("outline",
			func(_ context.Context, _ *core.ProcessContext, t seqTopic) (seqOutline, error) {
				return seqOutline{Sections: []string{"intro", t.Word, "conclusion"}}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[seqOutline](core.Goal{Description: "outline produced"})).
		Build()
	return a
}

// makeDraftAgent: takes seqOutline, produces seqDraft.
func makeDraftAgent() *core.Agent {
	a := agent.New("draft-agent").
		Description("expand an outline into a draft").
		Actions(agent.NewAction("draft",
			func(_ context.Context, _ *core.ProcessContext, o seqOutline) (seqDraft, error) {
				return seqDraft{Body: strings.Join(o.Sections, " | ")}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[seqDraft](core.Goal{Description: "draft produced"})).
		Build()
	return a
}

func TestSequenceAgents_TwoStepChain(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	outliner := makeOutlineAgent()
	drafter := makeDraftAgent()
	if err := platform.Deploy(outliner); err != nil {
		t.Fatalf("deploy outliner: %v", err)
	}
	if err := platform.Deploy(drafter); err != nil {
		t.Fatalf("deploy drafter: %v", err)
	}

	pipeline := workflow.SequenceAgents[seqTopic, seqDraft](
		platform, "topic-to-draft", outliner, drafter,
	)
	if err := platform.Deploy(pipeline); err != nil {
		t.Fatalf("deploy pipeline: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), pipeline,
		map[string]any{core.DefaultBindingName: seqTopic{Word: "agents"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}

	got, ok := core.ResultOfType[seqDraft](proc)
	if !ok {
		t.Fatal("no seqDraft bound on final blackboard")
	}
	want := "intro | agents | conclusion"
	if got.Body != want {
		t.Fatalf("Body = %q, want %q", got.Body, want)
	}
}

// makeFailingAgent returns an agent whose only action returns the given error.
func makeFailingAgent(name string, errMsg string) *core.Agent {
	return agent.New(name).
		Actions(agent.NewAction("failing",
			func(_ context.Context, _ *core.ProcessContext, t seqTopic) (seqOutline, error) {
				return seqOutline{}, fmt.Errorf("%s", errMsg)
			},
			core.ActionConfig{QoS: core.ActionQoS{MaxAttempts: 1}},
		)).
		Goals(agent.GoalProducing[seqOutline](core.Goal{Description: "outline (will fail)"})).
		Build()
}

func TestSequenceAgents_StepFailurePropagates(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	failing := makeFailingAgent("failing-step", "step blew up")
	drafter := makeDraftAgent()
	mustDeploy(t, platform, failing, drafter)

	pipeline := workflow.SequenceAgents[seqTopic, seqDraft](
		platform, "fail-pipeline", failing, drafter,
	)
	mustDeploy(t, platform, pipeline)

	proc, _ := platform.RunAgent(t.Context(), pipeline,
		map[string]any{core.DefaultBindingName: seqTopic{Word: "x"}},
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusFailed {
		t.Fatalf("status = %s; want StatusFailed", proc.Status())
	}
	if failure := proc.Failure(); failure == nil || !strings.Contains(failure.Error(), "step 0") {
		t.Fatalf("failure = %v; want one mentioning step 0", failure)
	}
}

func TestSequenceAgents_PanicsOnTooFewAgents(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.SequenceAgents[seqTopic, seqDraft](platform, "single", makeOutlineAgent())
}

func TestSequenceAgents_PanicsOnNilAgent(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.SequenceAgents[seqTopic, seqDraft](platform, "with-nil", makeOutlineAgent(), nil)
}

func TestSequenceAgents_PanicsOnNilPlatform(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.SequenceAgents[seqTopic, seqDraft](nil, "x", makeOutlineAgent(), makeDraftAgent())
}
