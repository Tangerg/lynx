package server

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// WorkspaceListFileChanges projects application VCS status onto the wire.
func (s *Server) WorkspaceListFileChanges(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	changes, err := s.workspaceVCS.ListFileChanges(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceFileChange, 0, len(changes))
	for _, change := range changes {
		entry := protocol.WorkspaceFileChange{
			Path: change.Path, Status: protocol.FileStatus(string(change.Status)), PreviousPath: change.PreviousPath, Binary: change.Binary,
		}
		if !change.Binary {
			added, removed := change.Added, change.Removed
			entry.Added, entry.Removed = &added, &removed
		}
		out = append(out, entry)
	}
	return protocol.NewPage(out), nil
}

// WorkspaceGetDiff validates wire-specific mode values then projects the
// application-owned diff onto the wire shape.
func (s *Server) WorkspaceGetDiff(ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
	base := false
	switch in.Mode {
	case "", protocol.DiffModeWorktree:
	case protocol.DiffModeBase:
		base = true
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", protocol.ErrInvalidParams, in.Mode)
	}
	diff, err := s.workspaceVCS.Diff(ctx, workspaceapp.DiffInput{
		Cwd: in.Cwd, Path: in.Path, Base: base, Raw: in.Format == protocol.DiffFormatRaw, Limit: in.Limit,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	if in.Format == protocol.DiffFormatRaw {
		return &protocol.Diff{Patch: diff.Patch}, nil
	}
	return &protocol.Diff{Files: diffFilesWire(diff.Files), Truncated: diff.Truncated}, nil
}

func diffFilesWire(files []workspaceapp.FileDiff) []protocol.FileDiff {
	out := make([]protocol.FileDiff, 0, len(files))
	for _, file := range files {
		entry := protocol.FileDiff{
			Path: file.Path, Status: protocol.FileStatus(string(file.Status)), PreviousPath: file.PreviousPath,
			Binary: file.Binary, Rows: diffRowsWire(file.Rows),
		}
		if !file.Binary {
			added, removed := file.Added, file.Removed
			entry.Added, entry.Removed = &added, &removed
		}
		out = append(out, entry)
	}
	return out
}

func diffRowsWire(rows []workspaceapp.DiffRow) []protocol.DiffRow {
	out := make([]protocol.DiffRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, protocol.DiffRow{
			Type: protocol.DiffRowType(string(row.Type)), Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, Code: row.Code,
		})
	}
	return out
}
