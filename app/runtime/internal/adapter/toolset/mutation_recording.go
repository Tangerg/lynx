package toolset

import (
	"context"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

type mutationRecorderKey struct{}

// WithMutationRecorder installs the invocation-scoped sink used by filesystem
// decorators to report writes that actually crossed their guards. Potential
// paths declared by a tool remain useful for locking and approval, but they are
// not evidence that a call changed the workspace.
func WithMutationRecorder(ctx context.Context, record func([]string)) context.Context {
	if record == nil {
		return ctx
	}
	return context.WithValue(ctx, mutationRecorderKey{}, record)
}

// withMutationRecording sits inside every soft guard and immediately outside
// the operation that may write. A read-before-write refusal never reaches it;
// an execution error never records; a successful mutation reports the paths
// declared by the tool while the invocation context is still alive.
func withMutationRecording(inner toolcontract.Tool) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		paths, pathErr := mutationPaths(inner, arguments)
		if pathErr != nil || len(paths) == 0 {
			return out, nil
		}
		if record, ok := ctx.Value(mutationRecorderKey{}).(func([]string)); ok {
			record(slices.Clone(paths))
		}
		return out, nil
	})
}
