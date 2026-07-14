// Package evaluation scores generated answers for retrieval-augmented
// generation workflows.
//
// Evaluator is a small consumer-side interface. FactEvaluator and
// RelevanceEvaluator use the minimal Core chat Model directly; they do not
// require a fluent client, executable tools, or a document object model.
package evaluation
