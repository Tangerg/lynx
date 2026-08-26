// Package evaluation defines provider-neutral evaluation inputs, reports, and
// evaluators.
//
// Evaluator is generic over the subject being evaluated. TextSample and the
// model-backed GroundednessEvaluator and AnswerRelevanceEvaluator cover common
// generated-text metrics without depending on RAG, retrieval, documents, or a
// particular storage implementation.
package evaluation
