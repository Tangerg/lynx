package server

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListWorkspaceFileChanges projects application VCS status onto the wire.
func (s *Server) ListWorkspaceFileChanges(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	changes, err := s.workspaceVCS.ListFileChanges(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceFileChange, 0, len(changes))
	for _, change := range changes {
		status, ok := presentFileStatus(change.Status)
		if !ok {
			return nil, fmt.Errorf("workspace.changes.list: unsupported file status %q", change.Status)
		}
		entry := protocol.WorkspaceFileChange{
			Path: change.Path, Status: status, PreviousPath: change.PreviousPath, Binary: change.Binary,
		}
		if !change.Binary {
			added, removed := change.Added, change.Removed
			entry.Added, entry.Removed = &added, &removed
		}
		out = append(out, entry)
	}
	return protocol.NewPage(out), nil
}

// GetWorkspaceDiff validates wire-specific mode values then projects the
// application-owned diff onto the wire shape.
func (s *Server) GetWorkspaceDiff(ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
	base := false
	switch in.Mode {
	case "", protocol.DiffModeWorktree:
	case protocol.DiffModeBase:
		base = true
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", protocol.ErrInvalidParams, in.Mode)
	}
	diff, err := s.workspaceVCS.Diff(ctx, workspaceapp.DiffInput{
		Cwd: in.Workspace.Path, Path: in.Path, Base: base, Raw: in.Format == protocol.DiffFormatRaw, Limit: in.Limit,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	if in.Format == protocol.DiffFormatRaw {
		return &protocol.Diff{Patch: diff.Patch}, nil
	}
	files, err := presentDiffFiles(diff.Files)
	if err != nil {
		return nil, err
	}
	return &protocol.Diff{Files: files, Truncated: diff.Truncated}, nil
}

func presentDiffFiles(files []workspaceapp.FileDiff) ([]protocol.FileDiff, error) {
	out := make([]protocol.FileDiff, 0, len(files))
	for _, file := range files {
		status, ok := presentFileStatus(file.Status)
		if !ok {
			return nil, fmt.Errorf("workspace.diff.get: unsupported file status %q", file.Status)
		}
		rows, err := presentDiffRows(file.Rows)
		if err != nil {
			return nil, err
		}
		entry := protocol.FileDiff{
			Path: file.Path, Status: status, PreviousPath: file.PreviousPath,
			Binary: file.Binary, Rows: rows,
		}
		if !file.Binary {
			added, removed := file.Added, file.Removed
			entry.Added, entry.Removed = &added, &removed
		}
		out = append(out, entry)
	}
	return out, nil
}

func presentDiffRows(rows []workspaceapp.DiffRow) ([]protocol.DiffRow, error) {
	out := make([]protocol.DiffRow, 0, len(rows))
	for _, row := range rows {
		kind, ok := presentDiffRowType(row.Type)
		if !ok {
			return nil, fmt.Errorf("workspace.diff.get: unsupported row type %q", row.Type)
		}
		out = append(out, protocol.DiffRow{
			Type: kind, Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, Code: row.Code,
		})
	}
	return out, nil
}

func presentFileStatus(status workspaceapp.FileStatus) (protocol.FileStatus, bool) {
	switch status {
	case workspaceapp.FileStatusAdded:
		return protocol.FileStatusAdded, true
	case workspaceapp.FileStatusModified:
		return protocol.FileStatusModified, true
	case workspaceapp.FileStatusDeleted:
		return protocol.FileStatusDeleted, true
	case workspaceapp.FileStatusRenamed:
		return protocol.FileStatusRenamed, true
	case workspaceapp.FileStatusUntracked:
		return protocol.FileStatusUntracked, true
	default:
		return "", false
	}
}

func presentDiffRowType(kind workspaceapp.DiffRowType) (protocol.DiffRowType, bool) {
	switch kind {
	case workspaceapp.DiffRowHunk:
		return protocol.DiffRowHunk, true
	case workspaceapp.DiffRowContext:
		return protocol.DiffRowContext, true
	case workspaceapp.DiffRowAdded:
		return protocol.DiffRowAdded, true
	case workspaceapp.DiffRowDeleted:
		return protocol.DiffRowDeleted, true
	default:
		return "", false
	}
}
