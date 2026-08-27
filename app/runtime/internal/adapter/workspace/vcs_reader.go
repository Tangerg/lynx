package workspace

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/infra/git"
)

// VCS adapts Git status and diff reads to the workspace application ports.
// It translates raw Git failures into application-level outcomes.
type VCS struct{}

func (VCS) Changes(ctx context.Context, root string, maxChanges int) ([]workspaceapp.FileChange, error) {
	changes, err := ListChanges(ctx, root, maxChanges)
	if err != nil {
		return nil, vcsError(err)
	}
	out := make([]workspaceapp.FileChange, 0, len(changes))
	for _, change := range changes {
		status, ok := fileStatus(change.Status)
		if !ok {
			return nil, fmt.Errorf("workspace: unsupported git status %q", change.Status)
		}
		out = append(out, workspaceapp.FileChange{
			Path: change.Path, Status: status, PreviousPath: change.PreviousPath,
			Binary: change.Binary, Added: change.Added, Removed: change.Removed,
		})
	}
	return out, nil
}

func (VCS) StructuredDiff(
	ctx context.Context,
	root, path string,
	base bool,
	maxFiles, maxRows, maxBytes int,
) (workspaceapp.StructuredDiffResult, error) {
	files, truncated, err := Diff(ctx, root, path, base, maxFiles, maxRows, maxBytes)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, vcsError(err)
	}
	out := make([]workspaceapp.FileDiff, 0, len(files))
	for _, file := range files {
		status, ok := fileStatus(file.Status)
		if !ok {
			return workspaceapp.StructuredDiffResult{}, fmt.Errorf("workspace: unsupported git status %q", file.Status)
		}
		rows := make([]workspaceapp.DiffRow, 0, len(file.Rows))
		for _, row := range file.Rows {
			kind, ok := diffRowType(row.Type)
			if !ok {
				return workspaceapp.StructuredDiffResult{}, fmt.Errorf("workspace: unsupported diff row type %q", row.Type)
			}
			rows = append(rows, workspaceapp.DiffRow{
				Type: kind, Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, Code: row.Code,
			})
		}
		out = append(out, workspaceapp.FileDiff{
			Path: file.Path, Status: status, PreviousPath: file.PreviousPath,
			Binary: file.Binary, Added: file.Added, Removed: file.Removed, Rows: rows,
		})
	}
	return workspaceapp.StructuredDiffResult{Files: out, Truncated: truncated}, nil
}

func (VCS) RawDiff(ctx context.Context, root, path string, base bool, maxBytes int) (string, error) {
	patch, err := RawDiff(ctx, root, path, base, maxBytes)
	return patch, vcsError(err)
}

func fileStatus(status git.Status) (workspaceapp.FileStatus, bool) {
	switch status {
	case git.StatusAdded:
		return workspaceapp.FileStatusAdded, true
	case git.StatusModified:
		return workspaceapp.FileStatusModified, true
	case git.StatusDeleted:
		return workspaceapp.FileStatusDeleted, true
	case git.StatusRenamed:
		return workspaceapp.FileStatusRenamed, true
	case git.StatusUntracked:
		return workspaceapp.FileStatusUntracked, true
	default:
		return "", false
	}
}

func diffRowType(kind git.RowType) (workspaceapp.DiffRowType, bool) {
	switch kind {
	case git.RowHunk:
		return workspaceapp.DiffRowHunk, true
	case git.RowContext:
		return workspaceapp.DiffRowContext, true
	case git.RowAdded:
		return workspaceapp.DiffRowAdded, true
	case git.RowDeleted:
		return workspaceapp.DiffRowDeleted, true
	default:
		return "", false
	}
}

func vcsError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
		return workspaceapp.ErrVCSUnavailable
	case errors.Is(err, git.ErrNoBase):
		return workspaceapp.ErrVCSBaseUnknown
	case errors.Is(err, git.ErrResultTooLarge):
		return workspaceapp.ErrVCSResultTooLarge
	default:
		return err
	}
}
