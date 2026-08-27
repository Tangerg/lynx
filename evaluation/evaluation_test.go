package evaluation_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

var testMetric = evaluation.MustMetric("test")

func TestModelEvaluatorConstructionValidatesConfiguration(t *testing.T) {
	if _, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("nil model error = %v", err)
	}
	model := &fakeModel{reply: `{"score":0.5}`}
	negative := evaluation.Score(-0.1)
	if _, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model, Threshold: &negative}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("negative threshold error = %v", err)
	}
	large := evaluation.Score(1.1)
	if _, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model, Threshold: &large}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("large threshold error = %v", err)
	}
	missing, err := chatclient.ParseTemplate("{{.Missing}}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model, PromptTemplate: missing}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("unknown field error = %v", err)
	}
	var typedNilModel *fakeModel
	if _, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: typedNilModel}); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("typed nil model error = %v", err)
	}
}

func TestGroundednessEvaluatorBuildsStructuredRequestAndDecodesResult(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.95,"feedback":"Fully supported."}`}
	evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.TextSample{
		Output: "the claim", Context: []string{"source one", "", "source two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.Score != 0.95 || result.Feedback != "Fully supported." {
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

func TestAnswerRelevanceEvaluatorSupportsCustomPromptAndThreshold(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.6,"feedback":"Partly relevant."}`}
	prompt, err := chatclient.ParseTemplate("Q={{.Input}} A={{.Output}} C={{.Context}}")
	if err != nil {
		t.Fatal(err)
	}
	threshold := evaluation.Score(0.8)
	evaluator, err := evaluation.NewAnswerRelevanceEvaluator(evaluation.ModelConfig{
		Model: model, Threshold: &threshold, PromptTemplate: prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.TextSample{
		Input: "question", Output: "answer", Context: []string{"source"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || result.Score != 0.6 || result.Feedback != "Partly relevant." {
		t.Fatalf("result = %#v", result)
	}
	if got := model.lastRequest().Messages[0].Text(); got != "Q=question A=answer C=source" {
		t.Fatalf("custom prompt = %q", got)
	}
}

func TestModelEvaluatorSupportsExplicitZeroThreshold(t *testing.T) {
	model := &fakeModel{reply: `{"score":0}`}
	threshold := evaluation.Score(0)
	evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model, Threshold: &threshold})
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluator.Evaluate(t.Context(), evaluation.TextSample{Output: "answer", Context: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.Score != 0 {
		t.Fatalf("result = %#v, want passing zero score at zero threshold", result)
	}
}

func TestModelEvaluatorsRejectMissingSemanticInputs(t *testing.T) {
	model := &fakeModel{reply: `{"score":0.5}`}
	groundedness, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []evaluation.TextSample{
		{Context: []string{"source"}},
		{Output: "answer"},
		{Output: "answer", Context: []string{"  "}},
	} {
		if _, evaluateErr := groundedness.Evaluate(t.Context(), sample); !errors.Is(evaluateErr, evaluation.ErrInvalidSample) {
			t.Fatalf("Groundedness Evaluate(%#v) error = %v", sample, evaluateErr)
		}
	}
	relevance, err := evaluation.NewAnswerRelevanceEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relevance.Evaluate(t.Context(), evaluation.TextSample{Output: "answer", Context: []string{"source"}}); !errors.Is(err, evaluation.ErrInvalidSample) {
		t.Fatalf("missing input error = %v", err)
	}
	if calls := model.callCount(); calls != 0 {
		t.Fatalf("model calls after invalid inputs = %d", calls)
	}
}

func TestModelEvaluatorPreservesCancellationAndModelErrors(t *testing.T) {
	modelErr := errors.New("model failed")
	model := &fakeModel{err: modelErr}
	evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	sample := evaluation.TextSample{Output: "answer", Context: []string{"source"}}
	if _, err := evaluator.Evaluate(t.Context(), sample); !errors.Is(err, modelErr) {
		t.Fatalf("model error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := model.callCount()
	if _, err := evaluator.Evaluate(ctx, sample); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if calls := model.callCount(); calls != before {
		t.Fatalf("model called after cancellation: before %d after %d", before, calls)
	}
}

func TestModelEvaluatorRejectsNilResponse(t *testing.T) {
	model := &fakeModel{nilResponse: true}
	evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evaluator.Evaluate(t.Context(), evaluation.TextSample{Output: "answer", Context: []string{"source"}})
	if !errors.Is(err, evaluation.ErrInvalidReport) || !errors.Is(err, chatclient.ErrInvalidOutput) {
		t.Fatalf("nil response error = %v", err)
	}
}

func TestModelEvaluatorRejectsInvalidStructuredResults(t *testing.T) {
	for _, reply := range []string{"YES", "5 out of 10", "", `{"score":"0.5"}`} {
		model := &fakeModel{reply: reply}
		evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
		if err != nil {
			t.Fatal(err)
		}
		_, err = evaluator.Evaluate(t.Context(), evaluation.TextSample{Output: "answer", Context: []string{"source"}})
		if err == nil {
			t.Fatalf("reply %q error = %v", reply, err)
		}
	}
	model := &fakeModel{reply: `{"score":2}`}
	evaluator, err := evaluation.NewGroundednessEvaluator(evaluation.ModelConfig{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evaluator.Evaluate(t.Context(), evaluation.TextSample{Output: "answer", Context: []string{"source"}})
	if !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("out-of-range score error = %v", err)
	}
}

func TestCompositeMergesValidatedReportsWithoutFlattenedMetadata(t *testing.T) {
	firstMetadata := metadata.Map{}
	if err := firstMetadata.Set("source", "first"); err != nil {
		t.Fatal(err)
	}
	evaluators := []evaluation.Evaluator[evaluation.TextSample]{
		evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1, Feedback: "good", Metadata: firstMetadata}, nil
		}),
		evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric, Passed: false, Score: 0.5, Feedback: "weak"}, nil
		}),
	}
	composite, err := evaluation.NewComposite(evaluators...)
	if err != nil {
		t.Fatal(err)
	}
	evaluators[0] = nil
	result, err := composite.Evaluate(t.Context(), evaluation.TextSample{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || result.Score != 0.75 || result.Feedback != "good\n\nweak" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Details) != 2 {
		t.Fatalf("details = %#v, want two child reports", result.Details)
	}
	result.Details[0].Metadata["source"][1] = 'X'
	if string(firstMetadata["source"]) != `"first"` {
		t.Fatalf("child metadata was aliased: %s", firstMetadata["source"])
	}
}

func TestCompositeValidatesConstructionErrorsAndSingleReportOwnership(t *testing.T) {
	if _, err := evaluation.NewComposite[evaluation.TextSample](); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("empty composite error = %v", err)
	}
	if _, err := evaluation.NewComposite[evaluation.TextSample](nil); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("nil evaluator error = %v", err)
	}
	var typedNilEvaluator evaluation.EvaluatorFunc[evaluation.TextSample]
	if _, err := evaluation.NewComposite[evaluation.TextSample](typedNilEvaluator); !errors.Is(err, evaluation.ErrInvalidConfig) {
		t.Fatalf("typed nil evaluator error = %v", err)
	}

	childErr := errors.New("child failed")
	composite, err := evaluation.NewComposite(evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
		return evaluation.Report{}, childErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, evaluateErr := composite.Evaluate(t.Context(), evaluation.TextSample{}); !errors.Is(evaluateErr, childErr) {
		t.Fatalf("child error = %v", evaluateErr)
	}

	composite, err = evaluation.NewComposite(evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
		return evaluation.Report{Metric: testMetric, Score: evaluation.Score(math.NaN())}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, evaluateErr := composite.Evaluate(t.Context(), evaluation.TextSample{}); !errors.Is(evaluateErr, evaluation.ErrInvalidReport) {
		t.Fatalf("invalid child result error = %v", evaluateErr)
	}

	childMetadata := metadata.Map{}
	if setErr := childMetadata.Set("value", 1); setErr != nil {
		t.Fatal(setErr)
	}
	composite, err = evaluation.NewComposite(evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
		return evaluation.Report{Metric: testMetric, Passed: true, Score: 1, Metadata: childMetadata}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := composite.Evaluate(t.Context(), evaluation.TextSample{})
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
		evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
			cancel()
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1}, nil
		}),
		evaluation.EvaluatorFunc[evaluation.TextSample](func(context.Context, evaluation.TextSample) (evaluation.Report, error) {
			secondCalled = true
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(ctx, evaluation.TextSample{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if secondCalled {
		t.Fatal("second evaluator ran after cancellation")
	}
}

func TestScoreAndReportValidation(t *testing.T) {
	for _, score := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := (evaluation.Report{Metric: testMetric, Score: evaluation.Score(score)}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
			t.Fatalf("score %v error = %v", score, err)
		}
	}
	badMetadata := metadata.Map{"key": []byte("not-json")}
	if err := (evaluation.Report{Metric: testMetric, Score: 0.5, Metadata: badMetadata}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("metadata error = %v", err)
	}
	if err := (evaluation.Report{Metric: testMetric, Score: 0.5}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (evaluation.Report{Score: 0.5}).Validate(); !errors.Is(err, evaluation.ErrInvalidMetric) {
		t.Fatalf("missing metric error = %v", err)
	}
	if score, err := evaluation.NewScore(0.75); err != nil || score.Float64() != 0.75 {
		t.Fatalf("NewScore = %v, %v", score, err)
	}
	if _, err := evaluation.NewScore(-1); !errors.Is(err, evaluation.ErrInvalidScore) {
		t.Fatalf("NewScore(-1) error = %v", err)
	}
}

func TestTextSampleOwnsContext(t *testing.T) {
	context := []string{"first"}
	sample := evaluation.NewTextSample("input", "output", context)
	context[0] = "changed"
	clone := sample.Clone()
	clone.Context[0] = "clone"
	if got := sample.ContextText(); got != "first" {
		t.Fatalf("ContextText = %q, want first", got)
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

func (f *fakeModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	if f.nilResponse {
		return nil, nil
	}
	if f.reply == "" {
		return &chat.Response{}, nil
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(f.reply))
	return &chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}}, nil
}

func (f *fakeModel) lastRequest() *chat.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.request
}

func (f *fakeModel) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
