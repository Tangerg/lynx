package flow

import (
	"context"
)

// Loop enables repetitive execution of a processor until a termination condition is met.
// Generic parameters I and O define the input and output types for the loop.
type Loop[I any, O any] struct {
	// processor is the node that will be executed in each iteration
	processor Processor[any, any]
	// terminator determines when to stop the loop based on iteration count, input, and output
	terminator func(context.Context, int, I, O) (bool, error)
}

// shouldTerminate determines if the loop should stop iterating.
// Returns true if terminator is nil (default to single iteration) or if terminator returns true.
// Also returns any error from the terminator function.
func (l *Loop[I, O]) shouldTerminate(ctx context.Context, iteration int, input I, output O) (bool, error) {
	if l.terminator == nil {
		return true, nil
	}
	err := l.processor.checkContextCancellation(ctx)
	if err != nil {
		return false, err
	}
	return l.terminator(ctx, iteration, input, output)
}

// run executes the loop until the termination condition is met.
// It tracks the iteration count and passes it to the terminator.
// Returns the final output and any error encountered during execution.
func (l *Loop[I, O]) run(ctx context.Context, input I) (output O, err error) {
	var iteration = 0
	for {
		output, err = l.processor.Run(ctx, input)
		if err != nil {
			return
		}
		shouldTerminate, err1 := l.shouldTerminate(ctx, iteration, input, output)
		if err1 != nil {
			return output, err1
		}
		if shouldTerminate {
			return
		}
		iteration++
	}
}

// Run implements the Node interface for Loop.
// It first validates the processor, then executes the loop logic.
func (l *Loop[I, O]) Run(ctx context.Context, input I) (o O, err error) {
	err = validateProcessor(l.processor)
	if err != nil {
		return
	}
	return l.run(ctx, input)
}

// WithTerminator sets the termination condition for the loop.
// The terminator function receives the context, iteration count, original input, and current output.
// Returns the Loop for chaining.
func (l *Loop[I, O]) WithTerminator(terminator func(context.Context, int, I, O) (bool, error)) *Loop[I, O] {
	l.terminator = terminator
	return l
}

// WithProcessor sets the processor for the loop.
// This processor will be executed in each iteration until the terminator returns true.
// Returns the Loop for chaining.
func (l *Loop[I, O]) WithProcessor(processor Processor[any, any]) *Loop[I, O] {
	l.processor = processor
	return l
}

// LoopBuilder helps construct a Loop node with a fluent API.
// It maintains references to both the parent flow and the loop being built.
type LoopBuilder struct {
	flow *Flow
	loop *Loop[any, any]
}

// WithTerminator sets the termination condition for the loop.
// Returns the LoopBuilder for chaining.
func (l *LoopBuilder) WithTerminator(terminator func(context.Context, int, any, any) (bool, error)) *LoopBuilder {
	l.loop.WithTerminator(terminator)
	return l
}

// WithProcessor sets the processor for the loop.
// Returns the LoopBuilder for chaining.
func (l *LoopBuilder) WithProcessor(processor Processor[any, any]) *LoopBuilder {
	l.loop.WithProcessor(processor)
	return l
}

// Then adds the constructed loop to the parent flow and returns the flow.
// This completes the loop construction and continues building the flow.
func (l *LoopBuilder) Then() *Flow {
	l.flow.Then(l.loop)
	return l.flow
}
