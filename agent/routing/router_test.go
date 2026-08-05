package routing_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
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
	return agent.New(agent.Config{Name: name, Description: "test agent " + name, Actions: []agent.Action{agent.NewAction("act-"+name, func(_ context.Context, _ *core.ProcessContext, in chooseIn) (chooseOut, error) {
		return chooseOut{Done: true}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[chooseOut](core.GoalConfig{Description: "test goal " + name})}})
}

func mustRouter(t *testing.T, engine *runtime.Engine, ranker routing.Ranker, config routing.Config) *routing.Router {
	t.Helper()
	router, err := routing.New(engine, ranker, config)
	if err != nil {
		t.Fatalf("routing.New: %v", err)
	}
	if router == nil {
		t.Fatal("routing.New returned nil without an error")
	}
	return router
}

func TestRouter_PicksHighestConfidence(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a1 := newAgent("alpha")
	a2 := newAgent("beta")
	for _, a := range []*core.Agent{a1, a2} {
		if _, err := engine.Deploy(t.Context(), a); err != nil {
			t.Fatalf("deploy %s: %v", a.Name(), err)
		}
	}

	auto := mustRouter(t, engine, &stubRanker{
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
	if _, err := engine.Deploy(t.Context(), newAgent("alpha")); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	auto := mustRouter(t, engine, &stubRanker{
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

// TestRouterChoiceKeepsRankedDeploymentAcrossRouteReplacement pins that a
// Choice is a snapshot of an exact immutable identity, not a name to resolve
// later: replacing the active route afterwards does not retarget it.
func TestRouterChoiceKeepsRankedDeploymentAcrossRouteReplacement(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	original, err := engine.Deploy(t.Context(), newAgent("stable"))
	if err != nil {
		t.Fatal(err)
	}
	router, err := routing.New(engine, &stubRanker{scores: map[string]float64{
		"stable:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 1,
	}}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}
	choice, err := router.Choose(t.Context(), "anything")
	if err != nil {
		t.Fatal(err)
	}

	replacement := agent.New(agent.Config{
		Name: "stable", Description: "replacement",
		Actions: newAgent("stable").Actions(), Goals: newAgent("stable").Goals(),
	})
	if _, err := engine.Replace(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	active, ok := engine.ActiveDeployment("stable")
	if !ok || active.Ref() == original.Ref() {
		t.Fatal("test did not replace the active route")
	}
	if choice.Deployment() != original.Ref() {
		t.Fatalf("choice deployment = %s, want ranked %s", choice.Deployment(), original.Ref())
	}
	// The caller runs the identity it was given, so the replacement cannot
	// retarget the work either.
	deployment, ok := engine.Deployment(choice.Deployment())
	if !ok {
		t.Fatalf("ranked deployment %s is no longer resolvable", choice.Deployment())
	}
	process, err := engine.RunDeployment(t.Context(), deployment, core.Input(chooseIn{Topic: "x"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if process.Deployment() != original.Ref() {
		t.Fatalf("process deployment = %s, want ranked %s", process.Deployment(), original.Ref())
	}
}

type droppingRanker struct{}

func (droppingRanker) Rank(context.Context, string, []routing.Candidate) ([]routing.Choice, error) {
	return nil, nil
}

func TestRouterRejectsRankerCandidateDrift(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(t.Context(), newAgent("alpha")); err != nil {
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

func TestRouterRejectsInvalidRankerConfidence(t *testing.T) {
	for _, confidence := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run(fmt.Sprint(confidence), func(t *testing.T) {
			engine := agent.MustNewEngine(runtime.Config{})
			mustDeploy(t, engine, newAgent("alpha"))
			router, err := routing.New(engine, &stubRanker{scores: map[string]float64{
				"alpha:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": confidence,
			}}, routing.Config{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = router.Choose(t.Context(), "anything")
			if err == nil || !strings.Contains(err.Error(), "confidence must be finite and between 0 and 1") {
				t.Fatalf("Choose confidence %v error = %v", confidence, err)
			}
		})
	}
}

func TestRouter_AgentFilter(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("public"), newAgent("internal"))

	auto := mustRouter(t, engine, &stubRanker{
		scores: map[string]float64{
			"public:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut":   0.5,
			"internal:produce_github.com/Tangerg/lynx/agent/routing_test.chooseOut": 0.99,
		},
	}, routing.Config{
		AgentFilter: func(a core.AgentDescriptor) bool { return a.Name() != "internal" },
	})

	candidates, err := auto.Candidates()
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
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
	deployment, err := engine.Deploy(t.Context(), definition)
	if err != nil {
		t.Fatal(err)
	}
	router, err := routing.New(engine, &stubRanker{}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := router.Candidates()
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidates[0]
	actions := candidate.Agent().Actions()
	actions[0] = core.ActionDescriptor{}
	goals := candidate.Agent().Goals()
	goals[0] = core.GoalDescriptor{}

	if candidate.Deployment() != deployment.Ref() ||
		candidate.Agent().Name() != deployment.Descriptor().Name() {
		t.Fatalf("candidate identity drifted: %s / %s", candidate.Deployment(), deployment.Ref())
	}
	if candidate.Goal().Name() == "" ||
		candidate.Agent().Actions()[0].Name() == "" ||
		candidate.Agent().Goals()[0].Name() == "" ||
		candidate.String() == "<invalid candidate>" {
		t.Fatal("candidate leaked definition mutation or lost its goal identity")
	}
}

func TestRouter_NoCandidatesError(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	auto := mustRouter(t, engine, &stubRanker{}, routing.Config{})

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
		{"typed nil ranker", func() error {
			_, err := routing.New(engine, (*stubRanker)(nil), routing.Config{})
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

type mutatingRanker struct{}

func (mutatingRanker) Rank(_ context.Context, _ string, candidates []routing.Candidate) ([]routing.Choice, error) {
	candidates[0], candidates[1] = candidates[1], candidates[0]
	choices := make([]routing.Choice, len(candidates))
	for index, candidate := range candidates {
		choices[index] = routing.Choice{Candidate: candidate, Confidence: 1}
	}
	return choices, nil
}

func TestRouterProtectsCandidateOrderFromRankerMutation(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"), newAgent("beta"))
	router, err := routing.New(engine, mutatingRanker{}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.Choose(t.Context(), "anything"); err == nil || !strings.Contains(err.Error(), "changed candidate") {
		t.Fatalf("Choose error = %v, want candidate-mutation rejection", err)
	}
}

type panickingRanker struct{ cause error }

func (r panickingRanker) Rank(context.Context, string, []routing.Candidate) ([]routing.Choice, error) {
	panic(r.cause)
}

func TestRouterContainsHostedCallbackPanics(t *testing.T) {
	cause := errors.New("routing callback panic")
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"))

	rankerRouter, err := routing.New(engine, panickingRanker{cause: cause}, routing.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rankerRouter.Choose(t.Context(), "anything"); !errors.Is(err, cause) || !strings.Contains(err.Error(), "Ranker panicked") {
		t.Fatalf("Choose error = %v, want attributed Ranker panic", err)
	}

	filterRouter, err := routing.New(engine, droppingRanker{}, routing.Config{
		AgentFilter: func(core.AgentDescriptor) bool { panic(cause) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidates, err := filterRouter.Candidates(); candidates != nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "AgentFilter panicked") {
		t.Fatalf("Candidates = %v, %v, want no partial result and attributed filter panic", candidates, err)
	}
}
