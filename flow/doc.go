// Package flow provides a flexible, type-safe framework for building data processing pipelines in Go.It allows developers to construct complex processing workflows by composing different types of processing nodes together using a fluent builder API.
//
// The core concept is based on nodes that transform input data to output data, which can be connected together to form processing pipelines.The package leverages Go's generics to provide type safety throughout the pipeline while maintaining flexibility in data types.
//
// This package is particularly useful for situations where data needs to go through multiple transformation steps, conditional processing, iterative operations, batch processing, or parallel execution. It simplifies the implementation of complex business logic by providing reusable building blocks that can be composed in various ways.
//
// Key features of the package include:
//
// - Type-safe processing through generics
// - Sequential processing with StepNode
// - Conditional branching with BranchNode
// - Iterative processing with LoopNode
// - Batch processing with BatchNode
// - Parallel execution with ParallelNode
// - Asynchronous processing with AsyncResult
// - Middleware support for cross-cutting concerns
// - Fluent builder API for readable pipeline construction
// - Composition of pipelines into larger processing structures
//
// The design follows a functional approach where processing logic is defined as functions that transform data, combined with structural patterns for organizing how these transformations are applied.
//
// Example usage:
//
// ```go
// // Create a simple processing pipeline
// pipeline := flow.NewFlow().
//
//	Step().
//	    WithProcessor(validateData).
//	Then().
//	Branch().
//	    WithRouteResolver(determineDataType).
//	    AddBranch("type1", processType1).
//	    AddBranch("type2", processType2).
//	    AddDefaultBranch(processOtherTypes).
//	Then()
//
// // Execute the pipeline
// result, err := pipeline.Run(ctx, inputData)
// ```
//
// For more complex scenarios, the package provides utility functions like `Chain` for connecting pre-defined nodes, `OfNode` for starting a flow from an existing node, and `OfProcessor` for starting from a simple processing function.
//
// The  is designed to handle errors gracefully throughout the pipeline and supports context propagation for cancellation and deadline awareness.
package flow
