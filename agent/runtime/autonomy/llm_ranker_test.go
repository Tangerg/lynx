package autonomy_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/runtime/autonomy"
	"github.com/Tangerg/lynx/core/model/chat"
)

// stubModel returns a fixed text reply for every Call. The reply is
// supposed to be JSON the ranker parses; tests configure it
// per-case.
type stubModel struct {
	defaults  *chat.Options
	reply     string
	gotPrompt string
}

func newStubModel(reply string) *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{defaults: opts, reply: reply}
}

func (m *stubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *stubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	// Capture the user prompt so tests can assert on what reached the model.
	for _, msg := range req.Messages {
		if msg.Type() == chat.MessageTypeUser {
			if u, ok := msg.(*chat.UserMessage); ok {
				m.gotPrompt = u.Text
			}
		}
	}
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.reply),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func TestLLMRanker_ParsesScoresAndRoutesToTopAgent(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	for _, name := range []string{"alpha", "beta"} {
		if err := platform.Deploy(newAgent(name)); err != nil {
			t.Fatalf("deploy %s: %v", name, err)
		}
	}

	_aut, _ := autonomy.New(platform, &stubRanker{}, autonomy.Config{})
	candidates := _aut.Candidates()
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	// Map.Iteration order is unspecified, so look up alpha/beta by
	// name rather than positional index when building the reply.
	var alphaCand, betaCand autonomy.Candidate
	for _, c := range candidates {
		switch c.Agent.Name {
		case "alpha":
			alphaCand = c
		case "beta":
			betaCand = c
		}
	}

	// Build a reply that scores beta higher.
	reply := `Here is the verdict:
{"choices":[
  {"id":"` + alphaCand.String() + `","confidence":0.2,"rationale":"weak"},
  {"id":"` + betaCand.String() + `","confidence":0.85,"rationale":"strong"}
]}
trailing prose ignored.`
	model := newStubModel(reply)
	client, err := chat.NewClient(model)
	if err != nil {
		t.Fatalf("NewClientWithModel: %v", err)
	}

	ranker, _ := autonomy.NewLLMRanker(client, autonomy.LLMRankerConfig{})
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
	for _, c := range choices {
		confidenceFor[c.Agent.Name] = c.Confidence
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

func TestLLMRanker_ClampsConfidence(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, newAgent("alpha"))

	_aut, _ := autonomy.New(platform, &stubRanker{}, autonomy.Config{})
	candidates := _aut.Candidates()
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":1.7,"rationale":"x"}]}`
	model := newStubModel(reply)
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMRanker(client, autonomy.LLMRankerConfig{})
	choices, err := ranker.Rank(t.Context(), "x", candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if choices[0].Confidence != 1.0 {
		t.Fatalf("expected clamped 1.0, got %f", choices[0].Confidence)
	}
}

func TestLLMRanker_MissingScoreDefaultsToZero(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, newAgent("alpha"), newAgent("beta"))

	_aut, _ := autonomy.New(platform, &stubRanker{}, autonomy.Config{})
	candidates := _aut.Candidates()
	// Reply scores only the first candidate; beta is omitted.
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":0.6,"rationale":""}]}`
	model := newStubModel(reply)
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMRanker(client, autonomy.LLMRankerConfig{})
	choices, err := ranker.Rank(t.Context(), "x", candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if choices[1].Confidence != 0 {
		t.Fatalf("missing score should default to 0, got %f", choices[1].Confidence)
	}
}

func TestLLMRanker_RejectsNonJSONReply(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, newAgent("alpha"))
	_aut, _ := autonomy.New(platform, &stubRanker{}, autonomy.Config{})
	candidates := _aut.Candidates()

	model := newStubModel("nope, no JSON at all here")
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMRanker(client, autonomy.LLMRankerConfig{})
	_, err := ranker.Rank(t.Context(), "x", candidates)
	if err == nil {
		t.Fatal("expected error on non-JSON reply")
	}
}

func TestLLMRanker_PromptIncludesGoalTagsAndExamples(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	// Build an agent whose goal carries Tags + Examples.
	taggedAgent := agent.New("tagged").
		Description("an agent for testing goal hints").
		Actions(agent.NewAction("act",
			func(_ context.Context, _ *core.ProcessContext, in chooseIn) (chooseOut, error) {
				return chooseOut{Done: true}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[chooseOut](core.Goal{
			Description: "categorize sentiment",
			Tags:        []string{"sentiment", "classifier"},
			Examples:    []string{"how do I feel about this?", "rate this review"},
		})).
		Build()
	if err := platform.Deploy(taggedAgent); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	_aut, _ := autonomy.New(platform, &stubRanker{}, autonomy.Config{})
	candidates := _aut.Candidates()
	reply := `{"choices":[{"id":"` + candidates[0].String() + `","confidence":1.0,"rationale":""}]}`
	model := newStubModel(reply)
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMRanker(client, autonomy.LLMRankerConfig{})
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

func TestLLMRanker_RejectsNilClient(t *testing.T) {
	if _, err := autonomy.NewLLMRanker(nil, autonomy.LLMRankerConfig{}); err == nil {
		t.Fatal("expected error")
	}
}
