package runtime

import (
	"context"
)

// RunChildWithState runs a child with a copy of the parent's entire blackboard.
// Use it only when the child needs the parent's working state. For ordinary
// delegation, prefer [Engine.RunChild], which copies ambient state only.
func (e *Engine) RunChildWithState(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childCopiesParentState,
	}.run()
}

// RunChild runs a child with a clean blackboard containing only the parent's
// protected ambient state. This is the safe default for self-contained
// delegation. The parent process must be attached to ctx with
// [core.WithProcessView].
func (e *Engine) RunChild(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}.run()
}

func runChildDeployment(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     engine,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}.run()
}

// RunChildIsolated runs a child with a fresh blackboard seeded only with input.
// Use it for loops, pipelines, and parallel branches that must not inherit even
// ambient state.
func (e *Engine) RunChildIsolated(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childStartsEmpty,
	}.run()
}
