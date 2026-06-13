package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/domain/workspace"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// workspace.* git-backed reads (AUX_API §2). Three states stay distinct
// throughout: no git binary → features.git=false (client never calls); git but
// non-repo → vcs_unavailable; repo + no changes → empty result.

// WorkspaceListFileChanges scans the cwd's working tree (AUX_API §2.2).
func (s *Server) WorkspaceListFileChanges(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	changes, err := workspace.ListChanges(ctx, root)
	if err != nil {
		return nil, mapGitErr(err)
	}
	out := make([]protocol.WorkspaceFileChange, 0, len(changes))
	for _, c := range changes {
		w := protocol.WorkspaceFileChange{
			Path: c.Path, Status: string(c.Status), PreviousPath: c.PreviousPath, Binary: c.Binary,
		}
		if !c.Binary {
			added, removed := c.Added, c.Removed
			w.Added, w.Removed = &added, &removed
		}
		out = append(out, w)
	}
	return protocol.NewPage(out), nil
}

// WorkspaceGetDiff returns the structured (rows) or raw (patch) diff for cwd
// (AUX_API §2.3). mode worktree|base; path is jailed to the root. limit caps
// rows, truncating at a file boundary.
func (s *Server) WorkspaceGetDiff(ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	rel := ""
	if in.Path != "" {
		if rel, err = resolveInRoot(root, in.Path); err != nil {
			return nil, err
		}
	}
	var base bool
	switch in.Mode {
	case "", "worktree":
	case "base":
		base = true
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", protocol.ErrInvalidParams, in.Mode)
	}

	if in.Format == "raw" {
		patch, err := workspace.RawDiff(ctx, root, rel, base)
		if err != nil {
			return nil, mapGitErr(err)
		}
		return &protocol.Diff{Patch: patch}, nil
	}
	files, err := workspace.Diff(ctx, root, rel, base)
	if err != nil {
		return nil, mapGitErr(err)
	}
	out, truncated := diffFilesToWire(files, in.Limit)
	return &protocol.Diff{Files: out, Truncated: truncated}, nil
}

// mapGitErr maps the git layer's typed errors onto wire sentinels (AUX_API
// §2.3): non-repo / unavailable → vcs_unavailable; unresolvable base branch →
// invalid_params (NOT vcs_unavailable — that's the "not a repo" signal).
func mapGitErr(err error) error {
	switch {
	case errors.Is(err, workspace.ErrNotRepo), errors.Is(err, workspace.ErrUnavailable):
		return protocol.ErrVcsUnavailable
	case errors.Is(err, workspace.ErrNoBase):
		return fmt.Errorf("%w: cannot resolve base branch", protocol.ErrInvalidParams)
	default:
		return err
	}
}

// diffFilesToWire maps git DiffFiles onto the wire, capping total rows at limit
// with a file-boundary cut (a file is included whole or not at all; a single
// over-limit file is still included so the client gets something).
func diffFilesToWire(files []workspace.DiffFile, limit int) ([]protocol.FileDiff, bool) {
	out := make([]protocol.FileDiff, 0, len(files))
	rows, truncated := 0, false
	for _, f := range files {
		if limit > 0 && len(out) > 0 && rows+len(f.Rows) > limit {
			truncated = true
			break
		}
		fd := protocol.FileDiff{
			Path: f.Path, Status: string(f.Status), PreviousPath: f.PreviousPath,
			Binary: f.Binary, Rows: rowsToWire(f.Rows),
		}
		if !f.Binary {
			added, removed := f.Added, f.Removed
			fd.Added, fd.Removed = &added, &removed
		}
		out = append(out, fd)
		rows += len(f.Rows)
	}
	return out, truncated
}

func rowsToWire(rows []workspace.Row) []protocol.DiffRow {
	out := make([]protocol.DiffRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, protocol.DiffRow{
			Type: r.Type, Text: r.Text, LeftLine: r.LeftLine, RightLine: r.RightLine, Code: r.Code,
		})
	}
	return out
}
