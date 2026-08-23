package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/tools/fs"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// ConfinedExecutor adapts the shared filesystem tools to one admitted physical
// workspace root. LocalExecutor deliberately is not a jail; this adapter is
// the single owner of lexical and symlink-aware confinement in app2.
type ConfinedExecutor struct {
	root     string
	delegate *fs.LocalExecutor
}

func NewConfinedExecutor(root string) (*ConfinedExecutor, error) {
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("workspacefs: resolve tool root: %w", err)
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return nil, fmt.Errorf("workspacefs: make tool root absolute: %w", err)
	}
	return &ConfinedExecutor{
		root:     filepath.Clean(physical),
		delegate: fs.NewLocalExecutor(physical),
	}, nil
}

// Path validates a filesystem argument against the admitted root and returns
// the normalized relative form expected by LocalExecutor. Existing targets and
// the nearest existing ancestor of new targets are both resolved physically.
func (executor *ConfinedExecutor) Path(path string) (string, error) {
	if executor == nil || executor.root == "" {
		return "", errors.New("workspacefs: confined executor is not initialized")
	}
	if strings.TrimSpace(path) == "" || containsParent(path) {
		return "", protocol.ErrPathOutsideRoot
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(executor.root, filepath.Clean(path))
		if err != nil || escapes(relative) {
			return "", protocol.ErrPathOutsideRoot
		}
		path = relative
	}

	candidate := filepath.Join(executor.root, filepath.Clean(path))
	for ancestor := candidate; ; ancestor = filepath.Dir(ancestor) {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			relative, relErr := filepath.Rel(executor.root, resolved)
			if relErr != nil || escapes(relative) {
				return "", protocol.ErrPathOutsideRoot
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("workspacefs: resolve tool path: %w", err)
		}
		if filepath.Dir(ancestor) == ancestor {
			return "", protocol.ErrPathOutsideRoot
		}
	}
	return filepath.ToSlash(filepath.Clean(path)), nil
}

func (executor *ConfinedExecutor) Read(ctx context.Context, input fs.ReadInput) (fs.ReadOutput, error) {
	path, err := executor.Path(input.Path)
	if err != nil {
		return fs.ReadOutput{}, err
	}
	input.Path = path
	return executor.delegate.Read(ctx, input)
}

func (executor *ConfinedExecutor) Write(ctx context.Context, input fs.WriteInput) (fs.WriteOutput, error) {
	path, err := executor.Path(input.Path)
	if err != nil {
		return fs.WriteOutput{}, err
	}
	input.Path = path
	return executor.delegate.Write(ctx, input)
}

func (executor *ConfinedExecutor) Edit(ctx context.Context, input fs.EditInput) (fs.EditOutput, error) {
	path, err := executor.Path(input.Path)
	if err != nil {
		return fs.EditOutput{}, err
	}
	input.Path = path
	return executor.delegate.Edit(ctx, input)
}

func (executor *ConfinedExecutor) ApplyPatch(ctx context.Context, input fs.ApplyPatchInput) (fs.ApplyPatchOutput, error) {
	return executor.delegate.ApplyPatch(ctx, input)
}

func (executor *ConfinedExecutor) Glob(ctx context.Context, input fs.GlobInput) (fs.GlobOutput, error) {
	if filepath.IsAbs(input.Pattern) || containsParent(input.Pattern) {
		return fs.GlobOutput{}, protocol.ErrPathOutsideRoot
	}
	if input.Root != "" {
		path, err := executor.Path(input.Root)
		if err != nil {
			return fs.GlobOutput{}, err
		}
		input.Root = path
	}
	return executor.delegate.Glob(ctx, input)
}

func (executor *ConfinedExecutor) Grep(ctx context.Context, input fs.GrepInput) (fs.GrepOutput, error) {
	if input.Root != "" {
		path, err := executor.Path(input.Root)
		if err != nil {
			return fs.GrepOutput{}, err
		}
		input.Root = path
	}
	return executor.delegate.Grep(ctx, input)
}

func containsParent(path string) bool {
	for _, part := range strings.FieldsFunc(filepath.ToSlash(path), func(value rune) bool {
		return value == '/'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func escapes(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

var _ fs.Executor = (*ConfinedExecutor)(nil)
