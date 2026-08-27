// Package evaluation defines provider-neutral evaluation inputs, reports, and
// evaluators.
//
// Evaluator is generic over the subject being evaluated. TextSample and the
// model-backed GroundednessEvaluator and AnswerRelevanceEvaluator cover common
// generated-text metrics. RetrievalSample and RetrievalEvaluator cover
// provider-neutral precision, recall, reciprocal-rank, and NDCG measurements.
// The package does not depend on RAG, documents, or a storage implementation.
package evaluation
