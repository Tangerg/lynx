package eval_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/eval"
)

func ExampleExperiment_Run() {
	metric, err := eval.NewMetric(eval.MetricConfig{
		Namespace: "example",
		Name:      "non_empty",
	})
	if err != nil {
		panic(err)
	}
	evaluator := eval.EvaluatorFunc[string](func(_ context.Context, subject string) (eval.Report, error) {
		return eval.Report{Metric: metric, Verdict: eval.VerdictPass}, nil
	})
	dataset, err := eval.NewDataset(
		eval.Case[string]{ID: "first", Subject: "answer"},
	)
	if err != nil {
		panic(err)
	}
	experiment, err := eval.NewExperiment(eval.ExperimentConfig[string]{
		Dataset: dataset, Evaluator: evaluator,
	})
	if err != nil {
		panic(err)
	}
	report, err := experiment.Run(context.Background())
	if err != nil {
		panic(err)
	}
	summary := report.Summary()

	fmt.Println(summary.Total, summary.Passed)
	// Output:
	// 1 1
}
