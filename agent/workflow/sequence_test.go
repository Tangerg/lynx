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
	a := agent.New(agent.AgentConfig{Name: "outline-agent", Description: "expand a topic into an outline", Actions: []agent.Action{agent.NewAction("outline", func(_ context.Context, _ *core.ProcessContext, t seqTopic) (seqOutline, error) {
		return seqOutline{Sections: []string{"intro", t.Word, "conclusion"}}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[seqOutline](core.GoalConfig{Description: "outline produced"})}})
	return a
}

// makeDraftAgent: takes seqOutline, produces seqDraft.
func makeDraftAgent() *core.Agent {
	a := agent.New(agent.AgentConfig{Name: "draft-agent", Description: "expand an outline into a draft", Actions: []agent.Action{agent.NewAction("draft", func(_ context.Context, _ *core.ProcessContext, o seqOutline) (seqDraft, error) {
		return seqDraft{Body: strings.Join(o.Sections, " | ")}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[seqDraft](core.GoalConfig{Description: "draft produced"})}})
	return a
}

func TestSequence_TwoStepChain(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	outliner := makeOutlineAgent()
	drafter := makeDraftAgent()
	if _, err := engine.Deploy(outliner); err != nil {
		t.Fatalf("deploy outliner: %v", err)
	}
	if _, err := engine.Deploy(drafter); err != nil {
		t.Fatalf("deploy drafter: %v", err)
	}

	pipeline, err := workflow.Sequence[seqTopic, seqDraft](
		engine, "topic-to-draft", outliner, drafter,
	)
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	_, err = engine.Deploy(pipeline)
	if err != nil {
		t.Fatalf("deploy pipeline: %v", err)
	}

	var proc *runtime.Process
	proc, err = engine.Run(t.Context(), pipeline,
		core.Input(seqTopic{Word: "agents"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}

	got, ok := core.Result[seqDraft](proc)
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
	return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("failing", func(_ context.Context, _ *core.ProcessContext, t seqTopic) (seqOutline, error) {
		return seqOutline{}, fmt.Errorf("%s", errMsg)
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[seqOutline](core.GoalConfig{Description: "outline (will fail)"})}})
}

func TestSequence_StepFailurePropagates(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	failing := makeFailingAgent("failing-step", "step blew up")
	drafter := makeDraftAgent()
	mustDeploy(t, engine, failing, drafter)

	pipeline, err := workflow.Sequence[seqTopic, seqDraft](
		engine, "fail-pipeline", failing, drafter,
	)
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	mustDeploy(t, engine, pipeline)

	proc, _ := engine.Run(t.Context(), pipeline,
		core.Input(seqTopic{Word: "x"}),
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusFailed {
		t.Fatalf("status = %s; want StatusFailed", proc.Status())
	}
	if failure := proc.Failure(); failure == nil || !strings.Contains(failure.Error(), "step 0") {
		t.Fatalf("failure = %v; want one mentioning step 0", failure)
	}
}

func TestSequence_RejectsTooFewAgents(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := workflow.Sequence[seqTopic, seqDraft](engine, "single", makeOutlineAgent()); err == nil {
		t.Fatal("expected error")
	}
}

func TestSequence_RejectsNilAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := workflow.Sequence[seqTopic, seqDraft](engine, "with-nil", makeOutlineAgent(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestSequence_RejectsNilEngine(t *testing.T) {
	if _, err := workflow.Sequence[seqTopic, seqDraft](nil, "x", makeOutlineAgent(), makeDraftAgent()); err == nil {
		t.Fatal("expected error")
	}
}
