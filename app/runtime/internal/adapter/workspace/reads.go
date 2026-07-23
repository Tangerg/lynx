package workspace

import (
	"context"
	"errors"
	"path/filepath"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
	"github.com/Tangerg/lynx/tools/fs"
)

// Reads adapts local filesystem browsing and content search to the workspace
// application ports.
type Reads struct{}

func (Reads) ListFiles(ctx context.Context, root string, options workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	entries, err := ListFiles(ctx, root, ListFilesOptions{
		Path: options.Path, Glob: options.Glob, Recursive: options.Recursive, IncludeIgnored: options.IncludeIgnored,
	})
	if errors.Is(err, ErrListingTooLarge) {
		return nil, workspaceapp.ErrFileListTooLarge
	}
	if err != nil {
		return nil, err
	}
	out := make([]workspaceapp.FileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, workspaceapp.FileEntry{
			Path: entry.Path, Name: entry.Name, Kind: workspaceapp.FileEntryKind(entry.Kind),
			SizeBytes: entry.SizeBytes, ModifiedAt: entry.ModifiedAt,
		})
	}
	return out, nil
}

func (Reads) ReadFile(ctx context.Context, root string, input workspaceapp.FileReadInput) (workspaceapp.FileReadResult, error) {
	read := fs.ReadInput{Path: input.Path, MaxBytes: input.MaxBytes}
	if input.StartLine > 0 {
		read.Offset = input.StartLine - 1
		if input.EndLine >= input.StartLine {
			read.Limit = input.EndLine - input.StartLine + 1
		}
	}
	out, err := fs.NewLocalExecutor(root).Read(ctx, read)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	return workspaceapp.FileReadResult{
		Content: out.Content, TotalLines: out.TotalLines, StartLine: out.StartLine,
		EndLine: out.EndLine, Truncated: out.Truncated,
	}, nil
}

func (Reads) Grep(ctx context.Context, root string, input workspaceapp.GrepInput) (workspaceapp.GrepResult, error) {
	searchRoot := root
	if input.Path != "" {
		searchRoot = filepath.Join(root, input.Path)
	}
	exec := fs.NewLocalExecutor(root)
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: input.Query, Root: searchRoot, MaxResults: input.Limit})
	if err != nil {
		return workspaceapp.GrepResult{}, err
	}
	matches := make([]workspaceapp.GrepMatch, 0, len(out.Matches))
	for _, match := range out.Matches {
		matches = append(matches, workspaceapp.GrepMatch{Path: match.Path, LineNumber: match.Line, Text: match.Text})
	}
	total := len(matches)
	if out.Truncated {
		if count, countErr := grepTotal(ctx, exec, input.Query, searchRoot); countErr == nil && count > total {
			total = count
		} else if total == input.Limit {
			total = input.Limit + 1
		}
	}
	return workspaceapp.GrepResult{Matches: matches, Total: total}, nil
}

func grepTotal(ctx context.Context, exec fs.Executor, pattern, root string) (int, error) {
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: pattern, Root: root, OutputMode: fs.GrepOutputCount})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, count := range out.Counts {
		total += count.Count
	}
	return total, nil
}

// VCS adapts git-backed status and diff reads to the workspace application
// ports. It translates raw git errors into application-level VCS outcomes.
type VCS struct{}

func (VCS) ListFileChanges(ctx context.Context, root string) ([]workspaceapp.FileChange, error) {
	changes, err := ListChanges(ctx, root)
	if err != nil {
		return nil, vcsError(err)
	}
	out := make([]workspaceapp.FileChange, 0, len(changes))
	for _, change := range changes {
		out = append(out, workspaceapp.FileChange{
			Path: change.Path, Status: workspaceapp.FileStatus(change.Status), PreviousPath: change.PreviousPath,
			Binary: change.Binary, Added: change.Added, Removed: change.Removed,
		})
	}
	return out, nil
}

func (VCS) StructuredDiff(ctx context.Context, root, path string, base bool) ([]workspaceapp.FileDiff, error) {
	files, err := Diff(ctx, root, path, base)
	if err != nil {
		return nil, vcsError(err)
	}
	out := make([]workspaceapp.FileDiff, 0, len(files))
	for _, file := range files {
		rows := make([]workspaceapp.DiffRow, 0, len(file.Rows))
		for _, row := range file.Rows {
			rows = append(rows, workspaceapp.DiffRow{
				Type: workspaceapp.DiffRowType(row.Type), Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, Code: row.Code,
			})
		}
		out = append(out, workspaceapp.FileDiff{
			Path: file.Path, Status: workspaceapp.FileStatus(file.Status), PreviousPath: file.PreviousPath,
			Binary: file.Binary, Added: file.Added, Removed: file.Removed, Rows: rows,
		})
	}
	return out, nil
}

func (VCS) RawDiff(ctx context.Context, root, path string, base bool) (string, error) {
	patch, err := RawDiff(ctx, root, path, base)
	return patch, vcsError(err)
}

func vcsError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
		return workspaceapp.ErrVCSUnavailable
	case errors.Is(err, git.ErrNoBase):
		return workspaceapp.ErrVCSBaseUnknown
	default:
		return err
	}
}
