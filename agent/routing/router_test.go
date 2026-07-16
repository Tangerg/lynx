package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/routing"
	"github.com/Tangerg/lynx/agent/runtime"
)

type chooseIn struct{ Topic string }
type chooseOut struct{ Done bool }

// stubRanker scores Candidates by a fixed map keyed on
// "<agent>:<goal>". Missing entries score 0.
type stubRanker struct {
	scores map[string]float64
}

func (s *stubRanker) Rank(_ context.Context, _ string, candidates []routing.Candidate) ([]routing.Choice, error) {
	out := make([]routing.Choice, len(candidates))
	for i, c := range candidates {
		out[i] = routing.Choice{Candidate: c, Confidence: s.scores[c.String()]}
	}
	return out, nil
}

func newAgent(name string) *core.Agent {
	return agent.New(agent.AgentConfig{Name: name, Description: "test agent " + name, Actions: []agent.Action{agent.NewAction("act-"+name, func(_ context.Context, _ *core.ProcessContext, in chooseIn) (chooseOut, error) {
		return chooseOut{Done: true}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[chooseOut](core.GoalConfig{Description: "test goal " + name})}})
}

func TestRouter_PicksHighestConfidence(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a1 := newAgent("alpha")
	a2 := newAgent("beta")
	for _, a := range []*core.Agent{a1, a2} {
		if _, err := engine.Deploy(a); err != nil {
			t.Fatalf("deploy %s: %v", a.Name(), err)
		}
	}

	auto, _ := routing.New(engine, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 0.3,
			"beta:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut":  0.9,
		},
	}, routing.Config{})

	choice, err := auto.Choose(t.Context(), "anything")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if choice.Agent().Name() != "beta" {
		t.Fatalf("expected beta, got %s", choice.Agent().Name())
	}
	if choice.Confidence != 0.9 {
		t.Fatalf("Confidence = %f, want 0.9", choice.Confidence)
	}
}

func TestRouter_LowConfidenceReturnsError(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(newAgent("alpha")); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	auto, _ := routing.New(engine, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 0.3,
		},
	}, routing.Config{
		MinConfidence: 0.5,
	})

	_, err := auto.Choose(t.Context(), "anything")
	if !errors.Is(err, routing.ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch, got %v", err)
	}
}

func TestRouter_RunInstallsTargetGoalApprover(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(newAgent("alpha")); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	auto, _ := routing.New(engine, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 0.9,
		},
	}, routing.Config{})

	choice, proc, err := auto.Run(t.Context(), "anything",
		map[string]any{core.DefaultBindingName: chooseIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if choice.Agent().Name() != "alpha" {
		t.Fatalf("Choose returned wrong agent: %s", choice.Agent().Name())
	}
	if proc == nil || proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %v; failure = %v", proc.Status(), proc.Failure())
	}
	if proc.Deployment() != choice.Deployment() {
		t.Fatalf("process deployment = %s, chosen deployment = %s", proc.Deployment(), choice.Deployment())
	}
	got, ok := core.Result[chooseOut](proc)
	if !ok || !got.Done {
		t.Fatalf("expected Done=true, got %+v ok=%v", got, ok)
	}
}

type droppingRanker struct{}

func (droppingRanker) Rank(context.Context, string, []routing.Candidate) ([]routing.Choice, error) {
	return nil, nil
}

func TestRouterRejectsRankerCandidateDrift(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(newAgent("alpha")); err != nil {
		t.Fatal(err)
	}
	router, err := routing.New(engine, droppingRanker{}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.Choose(t.Context(), "anything"); err == nil {
		t.Fatal("Choose accepted a ranker that dropped the deployment-bound candidate")
	}
}

func TestRouter_AgentFilter(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("public"), newAgent("internal"))

	auto, _ := routing.New(engine, &stubRanker{
		scores: map[string]float64{
			"public:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut":   0.5,
			"internal:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 0.99,
		},
	}, routing.Config{
		AgentFilter: func(a *core.Agent) bool { return a.Name() != "internal" },
	})

	candidates := auto.Candidates()
	if len(candidates) != 1 || candidates[0].Agent().Name() != "public" {
		t.Fatalf("AgentFilter not respected; candidates=%v", candidates)
	}

	choice, err := auto.Choose(t.Context(), "x")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if choice.Agent().Name() != "public" {
		t.Fatalf("expected filtered Choose to pick 'public', got %s", choice.Agent().Name())
	}
}

func TestCandidateKeepsExactImmutableIdentity(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	definition := newAgent("stable")
	deployment, err := engine.Deploy(definition)
	if err != nil {
		t.Fatal(err)
	}
	router, err := routing.New(engine, &stubRanker{}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}

	candidate := router.Candidates()[0]
	actions := candidate.Agent().Actions()
	actions[0] = nil

	if candidate.Deployment() != deployment.Ref() || candidate.Agent() != deployment.Agent() {
		t.Fatalf("candidate identity drifted: %s / %s", candidate.Deployment(), deployment.Ref())
	}
	if candidate.Goal() == nil || candidate.Agent().Actions()[0] == nil || candidate.String() == "<invalid candidate>" {
		t.Fatal("candidate leaked definition mutation or lost its goal identity")
	}
}

func TestRouter_NoCandidatesError(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	auto, _ := routing.New(engine, &stubRanker{}, routing.Config{})

	_, err := auto.Choose(t.Context(), "x")
	if err == nil {
		t.Fatal("expected error on empty candidate pool")
	}
}

func TestRouter_RejectsNilArgs(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"nil engine", func() error {
			_, err := routing.New(nil, &stubRanker{}, routing.Config{})
			return err
		}},
		{"nil ranker", func() error {
			_, err := routing.New(engine, nil, routing.Config{})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
