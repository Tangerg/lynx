package runtime

import (
	"context"
	"errors"
)

// StartChild starts a child in the background and returns immediately. Like
// [Engine.RunChild], it copies only the parent's protected ambient state. The
// returned channel receives the terminal run error, if any.
//
// The child outlives cancellation of the calling action. Callers own its
// lifecycle through [Engine.Process], [Engine.Kill], or [Engine.KillChildren].
func (e *Engine) StartChild(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, <-chan error, error) {
	if e == nil {
		return nil, nil, errors.New("start child: engine is nil")
	}
	deployment, err := e.ownedDeployment("start child", deployment)
	if err != nil {
		return nil, nil, err
	}
	return startChildDeployment(ctx, e, deployment, input)
}

func startChildDeployment(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, <-chan error, error) {
	run := childRun{
		ctx:        ctx,
		engine:     engine,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}
	child, err := run.create()
	if err != nil {
		return nil, nil, err
	}
	done := engine.ContinueAsync(context.WithoutCancel(ctx), child.ID())
	return child, done, nil
}
