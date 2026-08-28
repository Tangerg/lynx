package evaluation_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

const testMetric evaluation.Metric = "test"

func TestCompositeMergesValidatedReportsWithoutFlattenedMetadata(t *testing.T) {
	firstMetadata := metadata.Map{}
	if err := firstMetadata.Set("source", "first"); err != nil {
		t.Fatal(err)
	}
	evaluators := []evaluation.Evaluator[string]{
		evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{
				Metric: testMetric, Passed: true, Score: 1, Feedback: "good", Metadata: firstMetadata,
			}, nil
		}),
		evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric, Passed: false, Score: 0.5, Feedback: "weak"}, nil
		}),
	}
	composite, err := evaluation.NewCompositeEvaluator(evaluators...)
	if err != nil {
		t.Fatal(err)
	}
	evaluators[0] = nil
	result, err := composite.Evaluate(t.Context(), "subject")
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
	if _, err := evaluation.NewCompositeEvaluator[string](); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("empty composite error = %v", err)
	}
	if _, err := evaluation.NewCompositeEvaluator[string](nil); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("nil evaluator error = %v", err)
	}
	var typedNilEvaluator evaluation.EvaluatorFunc[string]
	if _, err := evaluation.NewCompositeEvaluator[string](typedNilEvaluator); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("typed nil evaluator error = %v", err)
	}

	childErr := errors.New("child failed")
	composite, err := evaluation.NewCompositeEvaluator(evaluation.EvaluatorFunc[string](
		func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{}, childErr
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, evaluateErr := composite.Evaluate(t.Context(), "subject"); !errors.Is(evaluateErr, childErr) {
		t.Fatalf("child error = %v", evaluateErr)
	}

	composite, err = evaluation.NewCompositeEvaluator(evaluation.EvaluatorFunc[string](
		func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric, Score: evaluation.Score(math.NaN())}, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, evaluateErr := composite.Evaluate(t.Context(), "subject"); !errors.Is(evaluateErr, evaluation.ErrInvalidReport) {
		t.Fatalf("invalid child result error = %v", evaluateErr)
	}

	childMetadata := metadata.Map{}
	if setErr := childMetadata.Set("value", 1); setErr != nil {
		t.Fatal(setErr)
	}
	composite, err = evaluation.NewCompositeEvaluator(evaluation.EvaluatorFunc[string](
		func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1, Metadata: childMetadata}, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	result, err := composite.Evaluate(t.Context(), "subject")
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
	composite, err := evaluation.NewCompositeEvaluator(
		evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			cancel()
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1}, nil
		}),
		evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			secondCalled = true
			return evaluation.Report{Metric: testMetric, Passed: true, Score: 1}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(ctx, "subject"); !errors.Is(err, context.Canceled) {
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
