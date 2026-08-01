package workflow

import "context"

// Generator is one concurrent branch of a fan-out workflow. It receives only
// cancellation and the workflow's typed input: parent-process state and
// lifecycle control are deliberately absent, so branch results can cross the
// join boundary only through the returned value.
//
// Every concurrently running generator receives the same input value. Treat
// reference-bearing inputs such as pointers, maps, and slices as read-only or
// synchronize access outside the workflow. Cancellation is cooperative;
// generators must return when ctx is done. External side effects are not
// rolled back when a sibling fails.
type Generator[In, Out any] func(ctx context.Context, input In) (Out, error)
