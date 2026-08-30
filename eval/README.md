# eval

`eval` is a subject-agnostic quality-evaluation kernel. `Evaluator` is generic
over what it judges, so the same runtime evaluates a generated answer, a
retrieval ranking, an agent trajectory, or anything else a caller can type.

It owns datasets, evaluators, reports, suites, experiments, comparison, and
aggregation. It does not own dataset persistence, artifact storage, an
experiment-tracking service, or a dashboard — those belong to a Host.

## Install

```bash
go get github.com/Tangerg/scope/eval
```

## Packages

| Package | Owns |
|---|---|
| `eval` | The kernel: `Evaluator`, `Metric`, `Report`, `Dataset`, `Experiment`, `Comparison` |
| `judge` | Model-backed judgment adapted into typed reports |
| `text` | Generated-text quality metrics |
| `ranking` | Provider-neutral ranking metrics |

A new domain implements `Evaluator` directly. It does not depend on
text-generation or ranking concepts, and it does not add primitives to the root
package.

## Running an experiment

```go
dataset, err := eval.NewDataset(cases...)
if err != nil {
    return err
}

experiment, err := eval.NewExperiment(eval.ExperimentConfig[Answer]{
    Dataset:   dataset,
    Evaluator: evaluator,
})
if err != nil {
    return err
}

report, err := experiment.Run(ctx)
```

Concurrency is bounded — `DefaultMaxConcurrency` unless configured — and the
limit is resolved once at construction, never left to the caller to remember.

## Reports say only what was measured

A `Report` carries an optional verdict, a normalized score, a raw measurement,
feedback, and child reports, each independently. An evaluator that only produces
a measurement or qualitative feedback does not get a pass threshold or a
verdict invented for it.

`Metric` identity includes name, unit, direction, and parameters. Reports with
different identities are never aggregated together, so two metrics that happen
to share a name but not a unit cannot collapse into one number.

`Report.Details` is a bounded tree: `MaxReportDepth` is enforced at every
construction, clone, JSON, and summary boundary.

## Composing evaluators

- `SuiteEvaluator` runs several evaluators and preserves heterogeneous results
  side by side.
- `CompositeEvaluator` aggregates comparable scored verdicts explicitly —
  weights, required components, and a `PassAll` / `PassAny` / `PassAtLeast`
  rule, all part of the metric identity.
- `ProjectionEvaluator` adapts an aggregate subject down to a narrow evaluator's
  input.

## Failure policy

`ErrorCollect` records a case failure and continues. `ErrorFailFast` cancels
unscheduled work but never erases case facts that already settled.

## Comparison

`ExperimentReport.Compare` reports exact aggregate deltas. It does not claim
statistical significance — that requires a model and enough inputs, and the
kernel has neither by default.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants behind these rules.
