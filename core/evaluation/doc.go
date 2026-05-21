// Package evaluation defines the [Evaluator] surface used by RAG and
// agent pipelines to score generated responses — relevancy,
// factuality, and any other criteria the application chooses.
// Concrete evaluators ([FactCheckingEvaluator], [RelevancyEvaluator])
// live in this package; combine several into one verdict via
// [CompositeEvaluator].
package evaluation
