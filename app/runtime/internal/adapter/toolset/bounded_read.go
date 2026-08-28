package toolset

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/tools/fs"
)

const (
	maxRuntimeReadFileBytes   int64 = 8 << 20
	maxRuntimeReadOutputBytes       = 1 << 20
	maxRuntimeReadLineBytes         = 1 << 20
)

var errRuntimeReadFileTooLarge = errors.New("toolset: read file exceeds the 8 MiB limit")

// runtimeReadExecutor declares Runtime's model-facing read envelope while the
// filesystem reader remains the sole owner of path authority and bounded I/O.
type runtimeReadExecutor struct {
	next fs.Reader
}

func newRuntimeReadTool(root string, reader fs.Reader) *fs.ReadTool {
	if reader == nil {
		reader = fs.NewLocalExecutor(root)
	}
	return fs.NewReadTool(runtimeReadExecutor{next: reader})
}

func (r runtimeReadExecutor) Read(ctx context.Context, input fs.ReadInput) (fs.ReadOutput, error) {
	if cause := context.Cause(ctx); cause != nil {
		return fs.ReadOutput{}, cause
	}
	input.MaxInputBytes = maxRuntimeReadFileBytes
	input.MaxLineBytes = maxRuntimeReadLineBytes
	input.MaxOutputBytes = maxRuntimeReadOutputBytes
	result, err := r.next.Read(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrFileTooLarge):
			return fs.ReadOutput{}, fmt.Errorf("%w: %w", errRuntimeReadFileTooLarge, err)
		case errors.Is(err, fs.ErrLineTooLarge):
			return fs.ReadOutput{}, fmt.Errorf(
				"toolset: read %s: line %d exceeds the 1 MiB limit", input.Path, fs.ReadLineNumber(err),
			)
		default:
			return fs.ReadOutput{}, err
		}
	}
	return result, nil
}
