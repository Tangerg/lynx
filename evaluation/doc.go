// Package evaluation defines a subject-agnostic quality-evaluation kernel.
// Evaluator is generic over the subject, Metric carries structured identity and
// measurement semantics, and Report independently represents an optional
// verdict, normalized quality score, raw numeric measurement, feedback, and
// child reports. SuiteEvaluator preserves heterogeneous results while
// CompositeEvaluator explicitly aggregates comparable scored verdicts. Runner
// executes identified cases with bounded concurrency and per-metric
// distributions. ProjectionEvaluator adapts aggregate datasets to narrow
// evaluator subjects without forcing a shared sample shape.
//
// Domain vocabularies live outside the kernel: judge supplies generic
// model-backed evaluation, text owns generated-text metrics, and retrieval owns
// ranking metrics. New domains implement Evaluator directly and do not depend
// on text-generation or retrieval concepts.
package evaluation
