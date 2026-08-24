package toolset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/textread"
	"github.com/Tangerg/lynx/tools/fs"
)

const (
	maxRuntimeReadFileBytes   int64 = 8 << 20
	maxRuntimeReadOutputBytes       = 1 << 20
	maxRuntimeReadLineBytes         = 1 << 20
)

var errRuntimeReadFileTooLarge = errors.New("toolset: read file exceeds the 8 MiB limit")

// runtimeReadExecutor overrides only the model-facing Read operation. The
// embedded executor continues to own every other fs operation, while Runtime
// owns the complete-file and model-result envelopes its read consumer needs.
type runtimeReadExecutor struct {
	fs.Executor
	root string
}

func newRuntimeReadTool(root string, executor fs.Executor) *fs.ReadTool {
	if executor == nil {
		executor = fs.NewLocalExecutor(root)
	}
	return fs.NewReadTool(runtimeReadExecutor{Executor: executor, root: root})
}

func (executor runtimeReadExecutor) Read(ctx context.Context, input fs.ReadInput) (_ fs.ReadOutput, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return fs.ReadOutput{}, cause
	}
	path, err := runtimeReadPath(executor.root, input.Path)
	if err != nil {
		return fs.ReadOutput{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return fs.ReadOutput{}, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	info, err := file.Stat()
	if err != nil {
		return fs.ReadOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return fs.ReadOutput{}, fmt.Errorf("toolset: read %s: unsupported file mode %s", input.Path, info.Mode().Type())
	}
	if info.Size() > maxRuntimeReadFileBytes {
		return fs.ReadOutput{}, fmt.Errorf("%w: %s uses %d bytes", errRuntimeReadFileTooLarge, input.Path, info.Size())
	}
	return scanRuntimeRead(ctx, file, input)
}

func scanRuntimeRead(ctx context.Context, file io.Reader, input fs.ReadInput) (fs.ReadOutput, error) {
	result, err := textread.Scan(ctx, file, textread.Options{
		InputBytes: maxRuntimeReadFileBytes, LineBytes: maxRuntimeReadLineBytes,
		OutputBytes: maxRuntimeReadOutputBytes, StartLine: input.Offset, MaxLines: input.Limit,
	})
	if err != nil {
		switch {
		case errors.Is(err, textread.ErrInputTooLarge):
			return fs.ReadOutput{}, fmt.Errorf("%w: %s grew while reading", errRuntimeReadFileTooLarge, input.Path)
		case errors.Is(err, textread.ErrLineTooLarge):
			return fs.ReadOutput{}, fmt.Errorf(
				"toolset: read %s: line %d exceeds the 1 MiB limit", input.Path, textread.LineNumber(err),
			)
		case errors.Is(err, textread.ErrInvalidText):
			return fs.ReadOutput{}, fs.ErrBinaryFile
		default:
			return fs.ReadOutput{}, fmt.Errorf("toolset: scan %s: %w", input.Path, err)
		}
	}
	return fs.ReadOutput{
		Content: result.Content, StartLine: result.StartLine, EndLine: result.EndLine,
		TotalLines: result.TotalLines, Truncated: result.Truncated,
	}, nil
}

func runtimeReadPath(root, path string) (string, error) {
	if path == "" {
		return "", fs.ErrEmptyPath
	}
	if root == "" || filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(root, path), nil
}
