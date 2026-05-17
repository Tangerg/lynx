package evaluation_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/evaluation"
	"github.com/Tangerg/lynx/core/model/chat"
)

// fakeChatModel is the mock used across the evaluation suite. Every
// Call returns the configured replyText; Stream is not exercised but
// satisfies the interface.
type fakeChatModel struct {
	defaults  *chat.Options
	replyText string
	err       error
}

func newFakeChatModel(t *testing.T, reply string) *fakeChatModel {
	t.Helper()
	defaults, err := chat.NewOptions("eval-fake")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeChatModel{defaults: defaults, replyText: reply}
}

func (m *fakeChatModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *fakeChatModel) Metadata() chat.ModelMetadata          { return chat.ModelMetadata{Provider: "fake"} }

func (m *fakeChatModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.replyText),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *fakeChatModel) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {}
}

// --- Composite ------------------------------------------------------------

func TestNewCompositeEvaluator_RejectsEmpty(t *testing.T) {
	if _, err := evaluation.NewCompositeEvaluator(); err == nil {
		t.Fatal("zero evaluators must error")
	}
}

func TestCompositeEvaluator_RejectsNilRequest(t *testing.T) {
	c, _ := evaluation.NewCompositeEvaluator(passEvaluator{})
	if _, err := c.Evaluate(context.Background(), nil); err == nil {
		t.Fatal("nil request must error")
	}
}

func TestCompositeEvaluator_AllPass(t *testing.T) {
	composite, _ := evaluation.NewCompositeEvaluator(passEvaluator{}, passEvaluator{})

	got, err := composite.Evaluate(context.Background(), &evaluation.Request{Prompt: "q", Generation: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pass {
		t.Fatal("composite must pass when every child passes")
	}
	if got.Score != 1.0 {
		t.Fatalf("Score = %f, want 1.0", got.Score)
	}
}

func TestCompositeEvaluator_AnyFailVetoes(t *testing.T) {
	composite, _ := evaluation.NewCompositeEvaluator(passEvaluator{}, failEvaluator{})

	got, err := composite.Evaluate(context.Background(), &evaluation.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pass {
		t.Fatal("any child failure must veto Pass")
	}
}

func TestCompositeEvaluator_PropagatesError(t *testing.T) {
	want := errors.New("evaluator broke")
	composite, _ := evaluation.NewCompositeEvaluator(passEvaluator{}, errEvaluator{err: want})

	if _, err := composite.Evaluate(context.Background(), &evaluation.Request{}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

// --- Relevancy ------------------------------------------------------------

func TestRelevancyEvaluator_RejectsNilConfig(t *testing.T) {
	if _, err := evaluation.NewRelevancyEvaluator(nil); err == nil {
		t.Fatal("nil config must error")
	}
}

func TestRelevancyEvaluator_RejectsMissingChatModel(t *testing.T) {
	if _, err := evaluation.NewRelevancyEvaluator(&evaluation.RelevancyEvaluatorConfig{}); err == nil {
		t.Fatal("missing ChatModel must error")
	}
}

func TestRelevancyEvaluator_PassOnYes(t *testing.T) {
	model := newFakeChatModel(t, "YES")
	eval, err := evaluation.NewRelevancyEvaluator(&evaluation.RelevancyEvaluatorConfig{ChatModel: model})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("context", nil)
	got, err := eval.Evaluate(context.Background(), &evaluation.Request{
		Prompt:     "q",
		Generation: "g",
		Documents:  []*document.Document{doc},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pass {
		t.Fatal("YES reply must produce Pass=true")
	}
}

func TestRelevancyEvaluator_FailOnNo(t *testing.T) {
	model := newFakeChatModel(t, "NO")
	eval, _ := evaluation.NewRelevancyEvaluator(&evaluation.RelevancyEvaluatorConfig{ChatModel: model})

	got, err := eval.Evaluate(context.Background(), &evaluation.Request{Prompt: "q", Generation: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pass {
		t.Fatal("NO reply must produce Pass=false")
	}
}

func TestRelevancyEvaluator_RejectsNilRequest(t *testing.T) {
	model := newFakeChatModel(t, "YES")
	eval, _ := evaluation.NewRelevancyEvaluator(&evaluation.RelevancyEvaluatorConfig{ChatModel: model})

	if _, err := eval.Evaluate(context.Background(), nil); err == nil {
		t.Fatal("nil request must error")
	}
}

// --- Fact-checking --------------------------------------------------------

func TestFactCheckingEvaluator_RejectsMissingChatModel(t *testing.T) {
	if _, err := evaluation.NewFactCheckingEvaluator(&evaluation.FactCheckingEvaluatorConfig{}); err == nil {
		t.Fatal("missing ChatModel must error")
	}
}

func TestFactCheckingEvaluator_PassOnYes(t *testing.T) {
	model := newFakeChatModel(t, "YES")
	eval, err := evaluation.NewFactCheckingEvaluator(&evaluation.FactCheckingEvaluatorConfig{ChatModel: model})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("source", nil)
	got, err := eval.Evaluate(context.Background(), &evaluation.Request{
		Generation: "claim",
		Documents:  []*document.Document{doc},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pass || got.Score != 1.0 {
		t.Fatalf("Pass=%v Score=%f, want true/1.0", got.Pass, got.Score)
	}
}

// --- Helpers --------------------------------------------------------------

type passEvaluator struct{}

func (passEvaluator) Evaluate(_ context.Context, _ *evaluation.Request) (*evaluation.Response, error) {
	return &evaluation.Response{Pass: true, Score: 1.0}, nil
}

type failEvaluator struct{}

func (failEvaluator) Evaluate(_ context.Context, _ *evaluation.Request) (*evaluation.Response, error) {
	return &evaluation.Response{Pass: false, Score: 0.0}, nil
}

type errEvaluator struct{ err error }

func (e errEvaluator) Evaluate(_ context.Context, _ *evaluation.Request) (*evaluation.Response, error) {
	return nil, e.err
}
