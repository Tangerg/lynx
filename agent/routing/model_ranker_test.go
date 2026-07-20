package routing_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/routing"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/chat"
)

// stubModel returns a fixed text reply for every Call. The reply is
// supposed to be JSON the ranker parses; tests configure it
// per-case.
type stubModel struct {
	reply     string
	gotPrompt string
}

func newStubModel(reply string) *stubModel {
	return &stubModel{reply: reply}
}

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	// Capture the user prompt so tests can assert on what reached the model.
	for _, msg := range req.Messages {
		if msg.Role == chat.RoleUser {
			m.gotPrompt = msg.Text()
		}
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	resp, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return resp, nil
}

func TestModelRanker_ParsesScoresAndRoutesToTopAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	for _, name := range []string{"alpha", "beta"} {
		if _, err := engine.Deploy(newAgent(name)); err != nil {
			t.Fatalf("deploy %s: %v", name, err)
		}
	}

	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	// Map.Iteration order is unspecified, so look up alpha/beta by
	// name rather than positional index when building the reply.
	var alphaCandidate, betaCandidate routing.Candidate
	for _, candidate := range candidates {
		switch candidate.Agent().Name() {
		case "alpha":
			alphaCandidate = candidate
		case "beta":
			betaCandidate = candidate
		}
	}

	// Build a reply that scores beta higher.
	reply := `Here is the verdict:
{"choices":[
  {"id":"` + alphaCandidate.String() + `","confidence":0.2,"rationale":"weak"},
  {"id":"` + betaCandidate.String() + `","confidence":0.85,"rationale":"strong"}
]}
	trailing prose ignored.`
	model := newStubModel(reply)
	ranker, _ := routing.NewModelRanker(model, routing.ModelConfig{})
	choices, err := ranker.Rank(t.Context(), "user wants beta", candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(choices) != 2 {
		t.Fatalf("expected 2 choices, got %d", len(choices))
	}
	// Choices are positionally aligned with input candidates — find
	// alpha/beta by name regardless of input order.
	confidenceFor := map[string]float64{}
	for _, choice := range choices {
		confidenceFor[choice.Agent().Name()] = choice.Confidence
	}
	if confidenceFor["alpha"] != 0.2 {
		t.Fatalf("alpha confidence = %f, want 0.2", confidenceFor["alpha"])
	}
	if confidenceFor["beta"] != 0.85 {
		t.Fatalf("beta confidence = %f, want 0.85", confidenceFor["beta"])
	}

	// Prompt should mention candidate IDs and the user input.
	if !strings.Contains(model.gotPrompt, "user wants beta") {
		t.Fatalf("user input missing from prompt: %s", model.gotPrompt)
	}
	if !strings.Contains(model.gotPrompt, candidates[0].String()) {
		t.Fatalf("candidate id missing from prompt: %s", model.gotPrompt)
	}
}

func TestModelRanker_RejectsOutOfRangeConfidence(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"))

	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":1.7,"rationale":"x"}]}`
	model := newStubModel(reply)
	ranker, _ := routing.NewModelRanker(model, routing.ModelConfig{})
	choices, err := ranker.Rank(t.Context(), "x", candidates)
	if err == nil {
		t.Fatalf("Rank = %#v, nil; want invalid-confidence error", choices)
	}
	if !strings.Contains(err.Error(), "confidence must be finite and between 0 and 1") {
		t.Fatalf("Rank error = %v, want confidence range detail", err)
	}
}

func TestModelRanker_MissingScoreDefaultsToZero(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"), newAgent("beta"))

	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()
	// Reply scores only the first candidate; beta is omitted.
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":0.6,"rationale":""}]}`
	model := newStubModel(reply)
	ranker, _ := routing.NewModelRanker(model, routing.ModelConfig{})
	choices, err := ranker.Rank(t.Context(), "x", candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if choices[1].Confidence != 0 {
		t.Fatalf("missing score should default to 0, got %f", choices[1].Confidence)
	}
}

func TestModelRanker_RejectsNonJSONReply(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"))
	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()

	model := newStubModel("nope, no JSON at all here")
	ranker, _ := routing.NewModelRanker(model, routing.ModelConfig{})
	_, err := ranker.Rank(t.Context(), "x", candidates)
	if err == nil {
		t.Fatal("expected error on non-JSON reply")
	}
}

func TestModelRankerRejectsAmbiguousCandidateIdentity(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, newAgent("alpha"))
	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()
	id := candidates[0].String()

	for _, test := range []struct {
		name  string
		reply string
		want  string
	}{
		{name: "duplicate", reply: `{"choices":[{"id":"` + id + `","confidence":0.5},{"id":"` + id + `","confidence":0.7}]}`, want: "appears more than once"},
		{name: "unknown", reply: `{"choices":[{"id":"unknown:goal","confidence":0.5}]}`, want: "unknown candidate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ranker, _ := routing.NewModelRanker(newStubModel(test.reply), routing.ModelConfig{})
			_, err := ranker.Rank(t.Context(), "x", candidates)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Rank error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestModelRanker_PromptIncludesGoalTagsAndExamples(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	// Build an agent whose goal carries Tags + Examples.
	taggedAgent := agent.New(agent.AgentConfig{Name: "tagged", Description: "an agent for testing goal hints", Actions: []agent.Action{agent.NewAction("act", func(_ context.Context, _ *core.ProcessContext, in chooseIn) (chooseOut, error) {
		return chooseOut{Done: true}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[chooseOut](core.GoalConfig{Description: "categorize sentiment", Tags: []string{"sentiment", "classifier"}, Examples: []string{"how do I feel about this?", "rate this review"}})}})
	if _, err := engine.Deploy(taggedAgent); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	router, _ := routing.New(engine, &stubRanker{}, routing.Config{})
	candidates := router.Candidates()
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":1.0,"rationale":""}]}`
	model := newStubModel(reply)
	ranker, _ := routing.NewModelRanker(model, routing.ModelConfig{})
	if _, err := ranker.Rank(t.Context(), "x", candidates); err != nil {
		t.Fatalf("Rank: %v", err)
	}

	// Verify the prompt surfaces tags + examples so an LLM ranker has the
	// fuller match signal.
	for _, want := range []string{
		"tags: sentiment, classifier",
		"examples:",
		`"how do I feel about this?"`,
		`"rate this review"`,
	} {
		if !strings.Contains(model.gotPrompt, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, model.gotPrompt)
		}
	}
}

func TestModelRanker_RejectsNilModel(t *testing.T) {
	if _, err := routing.NewModelRanker(nil, routing.ModelConfig{}); err == nil {
		t.Fatal("expected error")
	}
}
