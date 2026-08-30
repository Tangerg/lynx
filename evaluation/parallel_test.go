package evaluation

import (
	"context"
	"testing"
)

func TestEvaluateAllNormalizesNonPositiveConcurrency(t *testing.T) {
	evaluator := EvaluatorFunc[string](func(context.Context, string) (Report, error) {
		return Report{Metric: Metric{Name: "quality"}, Verdict: VerdictPass}, nil
	})
	reports, err := evaluateAll(t.Context(), []Evaluator[string]{evaluator}, 0, "subject")
	if err != nil {
		t.Fatalf("evaluateAll() error = %v", err)
	}
	if len(reports) != 1 || reports[0].Verdict != VerdictPass {
		t.Fatalf("evaluateAll() = %#v", reports)
	}
}
