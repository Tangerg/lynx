package autonomy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/runtime/autonomy"
)

type chooseIn struct{ Topic string }
type chooseOut struct{ Done bool }

// stubRanker scores Candidates by a fixed map keyed on
// "<agent>:<goal>". Missing entries score 0.
type stubRanker struct {
	scores map[string]float64
}

func (s *stubRanker) Rank(_ context.Context, _ string, candidates []autonomy.Candidate) ([]autonomy.Choice, error) {
	out := make([]autonomy.Choice, len(candidates))
	for i, c := range candidates {
		out[i] = autonomy.Choice{Candidate: c, Confidence: s.scores[c.String()]}
	}
	return out, nil
}

func newAgent(name string) *core.Agent {
	return agent.New(name).
		Description("test agent " + name).
		Actions(agent.NewAction("act-"+name,
			func(_ context.Context, _ *core.ProcessContext, in chooseIn) (chooseOut, error) {
				return chooseOut{Done: true}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[chooseOut](core.Goal{Description: "test goal " + name})).
		Build()
}

func TestAutonomy_PicksHighestConfidence(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a1 := newAgent("alpha")
	a2 := newAgent("beta")
	for _, a := range []*core.Agent{a1, a2} {
		if err := platform.Deploy(a); err != nil {
			t.Fatalf("deploy %s: %v", a.Name, err)
		}
	}

	auto := autonomy.New(platform, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut": 0.3,
			"beta:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut":  0.9,
		},
	}, autonomy.Config{})

	choice, err := auto.Choose(t.Context(), "anything")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if choice.Agent.Name != "beta" {
		t.Fatalf("expected beta, got %s", choice.Agent.Name)
	}
	if choice.Confidence != 0.9 {
		t.Fatalf("Confidence = %f, want 0.9", choice.Confidence)
	}
}

func TestAutonomy_LowConfidenceReturnsError(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(newAgent("alpha")); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	auto := autonomy.New(platform, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut": 0.3,
		},
	}, autonomy.Config{
		GoalConfidenceCutOff: 0.5,
	})

	_, err := auto.Choose(t.Context(), "anything")
	if !errors.Is(err, autonomy.ErrNoConfidentChoice) {
		t.Fatalf("expected ErrNoConfidentChoice, got %v", err)
	}
}

func TestAutonomy_RunInstallsTargetGoalApprover(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(newAgent("alpha")); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	auto := autonomy.New(platform, &stubRanker{
		scores: map[string]float64{
			"alpha:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut": 0.9,
		},
	}, autonomy.Config{})

	choice, proc, err := auto.Run(t.Context(), "anything",
		map[string]any{core.DefaultBindingName: chooseIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if choice.Agent.Name != "alpha" {
		t.Fatalf("Choose returned wrong agent: %s", choice.Agent.Name)
	}
	if proc == nil || proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %v; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[chooseOut](proc)
	if !ok || !got.Done {
		t.Fatalf("expected Done=true, got %+v ok=%v", got, ok)
	}
}

func TestAutonomy_AgentFilter(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, newAgent("public"), newAgent("internal"))

	auto := autonomy.New(platform, &stubRanker{
		scores: map[string]float64{
			"public:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut":   0.5,
			"internal:produce_github.com/Tangerg/lynx/agent/runtime/autonomy_test.chooseOut": 0.99,
		},
	}, autonomy.Config{
		AgentFilter: func(a *core.Agent) bool { return a.Name != "internal" },
	})

	candidates := auto.Candidates()
	if len(candidates) != 1 || candidates[0].Agent.Name != "public" {
		t.Fatalf("AgentFilter not respected; candidates=%v", candidates)
	}

	choice, err := auto.Choose(t.Context(), "x")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if choice.Agent.Name != "public" {
		t.Fatalf("expected filtered Choose to pick 'public', got %s", choice.Agent.Name)
	}
}

func TestAutonomy_NoCandidatesError(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	auto := autonomy.New(platform, &stubRanker{}, autonomy.Config{})

	_, err := auto.Choose(t.Context(), "x")
	if err == nil {
		t.Fatal("expected error on empty candidate pool")
	}
}

func TestAutonomy_PanicsOnNilArgs(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	for _, tc := range []struct {
		name string
		fn   func()
	}{
		{"nil platform", func() { autonomy.New(nil, &stubRanker{}, autonomy.Config{}) }},
		{"nil ranker", func() { autonomy.New(platform, nil, autonomy.Config{}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tc.fn()
		})
	}
}
