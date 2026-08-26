package evaluation_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/core/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/rag/evaluation"
)

func TestModelEvaluatorConstructionValidatesConfiguration(t *testing.T) {
	if _, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("nil model error = %v", err)
	}
	model := &fakeModel{reply: `{"score":0.5}`}
	negative := -0.1
	if _, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model, Threshold: &negative}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("negative threshold error = %v", err)
	}
	large := 1.1
	if _, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model, Threshold: &large}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("large threshold error = %v", err)
	}
	missing, err := chatclient.ParseTemplate("{{.Missing}}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model, PromptTemplate: missing}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("unknown field error = %v", err)
	}
	var typedNilModel *fakeModel
	if _, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: typedNilModel}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("typed nil model error = %v", err)
	}
}

func TestFactEvaluatorBuildsStructuredRequestAndDecodesResult(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.95,"feedback":"Fully supported."}`}
	evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.Request{
		Answer: "the claim", Context: []string{"source one", "", "source two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass || result.Score != 0.95 || result.Feedback != "Fully supported." {
		t.Fatalf("result = %#v", result)
	}
	request := model.lastRequest()
	if request == nil || len(request.Messages) != 1 || request.Messages[0].Role != chat.RoleUser {
		t.Fatalf("model request = %#v", request)
	}
	prompt := request.Messages[0].Text()
	for _, want := range []string{"source one\nsource two", "the claim"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt %q missing %q", prompt, want)
		}
	}
	if request.Options.OutputFormat == nil || request.Options.OutputFormat.Type != chat.OutputFormatJSONSchema {
		t.Fatalf("output format = %#v, want JSON Schema", request.Options.OutputFormat)
	}
}

func TestRelevanceEvaluatorSupportsCustomPromptAndThreshold(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.6,"feedback":"Partly relevant."}`}
	prompt, err := chatclient.ParseTemplate("Q={{.Query}} A={{.Answer}} C={{.Context}}")
	if err != nil {
		t.Fatal(err)
	}
	threshold := 0.8
	evaluator, err := evaluation.NewRelevanceEvaluator(evaluation.ModelConfig{
		Model: model, Threshold: &threshold, PromptTemplate: prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.Request{
		Query: "question", Answer: "answer", Context: []string{"source"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pass || result.Score != 0.6 || result.Feedback != "Partly relevant." {
		t.Fatalf("result = %#v", result)
	}
	if got := model.lastRequest().Messages[0].Text(); got != "Q=question A=answer C=source" {
		t.Fatalf("custom prompt = %q", got)
	}
}

func TestModelEvaluatorSupportsExplicitZeroThreshold(t *testing.T) {
	model := &fakeModel{reply: `{"score":0}`}
	threshold := 0.0
	evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model, Threshold: &threshold})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.Request{Answer: "answer", Context: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass || result.Score != 0 {
		t.Fatalf("result = %#v, want passing zero score at zero threshold", result)
	}
}

func TestModelEvaluatorsRejectMissingSemanticInputs(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.5}`}
	fact, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []evaluation.Request{
		{Context: []string{"source"}},
		{Answer: "answer"},
		{Answer: "answer", Context: []string{"  "}},
	} {
		if _, err := fact.Evaluate(t.Context(), request); !errors.Is(err, evaluation.ErrInvalidRequest) {
			t.Fatalf("Fact Evaluate(%#v) error = %v", request, err)
		}
	}
	relevance, err := evaluation.NewRelevanceEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relevance.Evaluate(t.Context(), evaluation.Request{Answer: "answer", Context: []string{"source"}}); !errors.Is(err, evaluation.ErrInvalidRequest) {
		t.Fatalf("missing query error = %v", err)
	}
	if calls := model.callCount(); calls != 0 {
		t.Fatalf("model calls after invalid inputs = %d", calls)
	}
}

func TestModelEvaluatorPreservesCancellationAndModelErrors(t *testing.T) {
	modelErr := errors.New("model failed")
	model := &fakeModel{err: modelErr}
	evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	request := evaluation.Request{Answer: "answer", Context: []string{"source"}}
	if _, err := evaluator.Evaluate(t.Context(), request); !errors.Is(err, modelErr) {
		t.Fatalf("model error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := model.callCount()
	if _, err := evaluator.Evaluate(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if calls := model.callCount(); calls != before {
		t.Fatalf("model called after cancellation: before %d after %d", before, calls)
	}
}

func TestModelEvaluatorRejectsNilResponse(t *testing.T) {
	model := &fakeModel{nilResponse: true}
	evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evaluator.Evaluate(t.Context(), evaluation.Request{Answer: "answer", Context: []string{"source"}})
	if !errors.Is(err, chatclient.ErrInvalidOutputFormat) {
		t.Fatalf("nil response error = %v", err)
	}
}

func TestModelEvaluatorRejectsInvalidStructuredResults(t *testing.T) {
	for _, reply := range []string{"YES", "5 out of 10", "", `{"score":"0.5"}`} {
		model := &fakeModel{reply: reply}
		evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
		if err != nil {
			t.Fatal(err)
		}
		_, err = evaluator.Evaluate(t.Context(), evaluation.Request{Answer: "answer", Context: []string{"source"}})
		if err == nil {
			t.Fatalf("reply %q error = %v", reply, err)
		}
	}
	model := &fakeModel{reply: `{"score":2}`}
	evaluator, err := evaluation.NewFactEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evaluator.Evaluate(t.Context(), evaluation.Request{Answer: "answer", Context: []string{"source"}})
	if !errors.Is(err, evaluation.ErrInvalidResult) {
		t.Fatalf("out-of-range score error = %v", err)
	}
}

func TestCompositeMergesValidatedResultsWithoutAliasing(t *testing.T) {
	firstMetadata := metadata.Map{}
	if err := firstMetadata.Set("source", "first"); err != nil {
		t.Fatal(err)
	}
	evaluators := []evaluation.Evaluator{
		evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
			return evaluation.Result{Pass: true, Score: 1, Feedback: "good", Metadata: firstMetadata}, nil
		}),
		evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
			return evaluation.Result{Pass: false, Score: 0.5, Feedback: "weak"}, nil
		}),
	}
	composite, err := evaluation.NewComposite(evaluators...)
	if err != nil {
		t.Fatal(err)
	}
	evaluators[0] = nil
	result, err := composite.Evaluate(t.Context(), evaluation.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pass || result.Score != 0.75 || result.Feedback != "[Evaluation 1] good\n\n[Evaluation 2] weak" {
		t.Fatalf("result = %#v", result)
	}
	if got, ok, err := result.Metadata.Decode[int]("total_evaluations"); err != nil || !ok || got != 2 {
		t.Fatalf("total_evaluations = %d/%v/%v", got, ok, err)
	}
	if got, ok, err := result.Metadata.Decode[int]("passed_count"); err != nil || !ok || got != 1 {
		t.Fatalf("passed_count = %d/%v/%v", got, ok, err)
	}
	result.Metadata["evaluation_1_source"][1] = 'X'
	if string(firstMetadata["source"]) != `"first"` {
		t.Fatalf("child metadata was aliased: %s", firstMetadata["source"])
	}
}

func TestCompositeValidatesConstructionErrorsAndSingleResultOwnership(t *testing.T) {
	if _, err := evaluation.NewComposite(); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("empty composite error = %v", err)
	}
	if _, err := evaluation.NewComposite(nil); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("nil evaluator error = %v", err)
	}
	var typedNilEvaluator evaluation.EvaluatorFunc
	if _, err := evaluation.NewComposite(typedNilEvaluator); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("typed nil evaluator error = %v", err)
	}

	childErr := errors.New("child failed")
	composite, err := evaluation.NewComposite(evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
		return evaluation.Result{}, childErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(t.Context(), evaluation.Request{}); !errors.Is(err, childErr) {
		t.Fatalf("child error = %v", err)
	}

	composite, err = evaluation.NewComposite(evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
		return evaluation.Result{Score: math.NaN()}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(t.Context(), evaluation.Request{}); !errors.Is(err, evaluation.ErrInvalidResult) {
		t.Fatalf("invalid child result error = %v", err)
	}

	childMetadata := metadata.Map{}
	if err := childMetadata.Set("value", 1); err != nil {
		t.Fatal(err)
	}
	composite, err = evaluation.NewComposite(evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
		return evaluation.Result{Pass: true, Score: 1, Metadata: childMetadata}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := composite.Evaluate(t.Context(), evaluation.Request{})
	if err != nil {
		t.Fatal(err)
	}
	result.Metadata["value"][0] = '9'
	if string(childMetadata["value"]) != "1" {
		t.Fatalf("single result metadata was aliased: %s", childMetadata["value"])
	}
}

func TestCompositePreservesContextCancellationBetweenChildren(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	secondCalled := false
	composite, err := evaluation.NewComposite(
		evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
			cancel()
			return evaluation.Result{Pass: true, Score: 1}, nil
		}),
		evaluation.EvaluatorFunc(func(context.Context, evaluation.Request) (evaluation.Result, error) {
			secondCalled = true
			return evaluation.Result{Pass: true, Score: 1}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(ctx, evaluation.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if secondCalled {
		t.Fatal("second evaluator ran after cancellation")
	}
}

func TestResultValidation(t *testing.T) {
	for _, score := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := (evaluation.Result{Score: score}).Validate(); !errors.Is(err, evaluation.ErrInvalidResult) {
			t.Fatalf("score %v error = %v", score, err)
		}
	}
	badMetadata := metadata.Map{"key": []byte("not-json")}
	if err := (evaluation.Result{Score: 0.5, Metadata: badMetadata}).Validate(); !errors.Is(err, evaluation.ErrInvalidResult) {
		t.Fatalf("metadata error = %v", err)
	}
	if err := (evaluation.Result{Score: 0.5}).Validate(); err != nil {
		t.Fatal(err)
	}
}

type fakeModel struct {
	mu          sync.Mutex
	reply       string
	err         error
	nilResponse bool
	request     *chat.Request
	calls       int
}

func (m *fakeModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.request = request
	if m.err != nil {
		return nil, m.err
	}
	if m.nilResponse {
		return nil, nil
	}
	if m.reply == "" {
		return &chat.Response{}, nil
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	return &chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}}, nil
}

func (m *fakeModel) lastRequest() *chat.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.request
}

func (m *fakeModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
