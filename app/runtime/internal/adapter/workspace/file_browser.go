package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/tools/fs"
)

// FileBrowser adapts local filesystem browsing and content search to the workspace
// application ports.
type FileBrowser struct{}

func (FileBrowser) List(ctx context.Context, root string, options workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	entries, err := ListFiles(ctx, root, ListFilesOptions{
		Path: options.Path, Glob: options.Glob, Recursive: options.Recursive, IncludeIgnored: options.IncludeIgnored,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrListingTooLarge):
			return nil, workspaceapp.ErrFileListTooLarge
		case errors.Is(err, ErrInvalidGlob):
			return nil, workspaceapp.ErrInvalidFileGlob
		default:
			return nil, err
		}
	}
	out := make([]workspaceapp.FileEntry, 0, len(entries))
	for _, entry := range entries {
		kind, ok := fileEntryKind(entry.Kind)
		if !ok {
			return nil, fmt.Errorf("workspace: unsupported file entry kind %q", entry.Kind)
		}
		out = append(out, workspaceapp.FileEntry{
			Path: entry.Path, Name: entry.Name, Kind: kind,
			SizeBytes: entry.SizeBytes, ModifiedAt: entry.ModifiedAt,
		})
	}
	return out, nil
}

func (FileBrowser) Read(ctx context.Context, root string, input workspaceapp.FileReadInput) (workspaceapp.FileReadResult, error) {
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

func (FileBrowser) Grep(ctx context.Context, root string, input workspaceapp.GrepInput) (workspaceapp.GrepResult, error) {
	searchRoot := root
	if input.Path != "" {
		searchRoot = filepath.Join(root, input.Path)
	}
	exec := fs.NewLocalExecutor(root)
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: input.Query, Root: searchRoot, MaxResults: input.Limit})
	if err != nil {
		return workspaceapp.GrepResult{}, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.GrepResult{}, fmt.Errorf("workspace: resolve grep root: %w", err)
	}
	matches := make([]workspaceapp.GrepMatch, 0, len(out.Matches))
	for _, match := range out.Matches {
		path, pathErr := rootRelativeGrepPath(root, canonicalRoot, match.Path)
		if pathErr != nil {
			return workspaceapp.GrepResult{}, pathErr
		}
		matches = append(matches, workspaceapp.GrepMatch{Path: path, LineNumber: match.Line, Text: match.Text})
	}
	slices.SortFunc(matches, func(a, b workspaceapp.GrepMatch) int {
		if order := cmp.Compare(a.Path, b.Path); order != 0 {
			return order
		}
		if order := cmp.Compare(a.LineNumber, b.LineNumber); order != 0 {
			return order
		}
		return cmp.Compare(a.Text, b.Text)
	})
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

// rootRelativeGrepPath converts the filesystem executor's host path back into
// the workspace identity promised by workspace.files.search. The executor may
// canonicalize macOS /var to /private/var, so both sides are resolved before
// containment is checked. No host-absolute path is allowed to cross this port.
func rootRelativeGrepPath(root, canonicalRoot, candidate string) (string, error) {
	abs := candidate
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, candidate)
	}
	canonicalCandidate, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve grep match %q: %w", candidate, err)
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("workspace: relativize grep match %q: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return filepath.ToSlash(rel), nil
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

func fileEntryKind(kind EntryKind) (workspaceapp.FileEntryKind, bool) {
	switch kind {
	case EntryFile:
		return workspaceapp.FileEntryFile, true
	case EntryDir:
		return workspaceapp.FileEntryDir, true
	case EntrySymlink:
		return workspaceapp.FileEntrySymlink, true
	default:
		return "", false
	}
}
