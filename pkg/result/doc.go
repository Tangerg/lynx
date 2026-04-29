// Package result provides a generic [Result] type that pairs a value
// with an error, useful for fluent pipelines and lazy error handling.
//
// Construct results with [New], [Value], or [Error]. Inspect them with
// the methods [Result.Get], [Result.Value], [Result.Error]. Use [Map]
// to transform a successful value while propagating errors unchanged.
package result
