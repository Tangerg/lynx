package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/textread"
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

func (FileBrowser) Read(ctx context.Context, root string, input workspaceapp.FileReadInput) (_ workspaceapp.FileReadResult, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return workspaceapp.FileReadResult{}, cause
	}
	budget, err := workspaceReadBudget(input.MaxBytes)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	path := input.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	info, err := file.Stat()
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	if !info.Mode().IsRegular() {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: read %s: unsupported file mode %s", input.Path, info.Mode().Type())
	}
	if info.Size() > workspaceapp.MaxFileReadSourceBytes {
		return workspaceapp.FileReadResult{}, fmt.Errorf(
			"%w: %s uses %d bytes", workspaceapp.ErrFileReadTooLarge, input.Path, info.Size(),
		)
	}

	start, lines := 0, 0
	if input.StartLine > 0 {
		start = input.StartLine - 1
		if input.EndLine >= input.StartLine {
			lines = input.EndLine - input.StartLine + 1
		}
	}
	result, err := textread.Scan(ctx, file, textread.Options{
		InputBytes: workspaceapp.MaxFileReadSourceBytes, LineBytes: workspaceapp.MaxFileReadLineBytes,
		OutputBytes: budget, StartLine: start, MaxLines: lines, PartialLine: true,
	})
	if err != nil {
		switch {
		case errors.Is(err, textread.ErrInputTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s grew while reading", workspaceapp.ErrFileReadTooLarge, input.Path)
		case errors.Is(err, textread.ErrLineTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf(
				"%w: %s line %d exceeds the 8 MiB limit",
				workspaceapp.ErrFileReadTooLarge, input.Path, textread.LineNumber(err),
			)
		case errors.Is(err, textread.ErrInvalidText):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s", workspaceapp.ErrUnsupportedText, input.Path)
		default:
			return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: scan %s: %w", input.Path, err)
		}
	}
	if input.StartLine > result.TotalLines {
		return workspaceapp.FileReadResult{}, workspaceapp.ErrInvalidFileRange
	}
	return workspaceapp.FileReadResult{
		Content: result.Content, TotalLines: result.TotalLines, StartLine: result.StartLine,
		EndLine: result.EndLine, Truncated: result.Truncated, OutputTruncated: result.OutputTruncated,
	}, nil
}

func workspaceReadBudget(requested int) (int, error) {
	switch {
	case requested < 0:
		return 0, workspaceapp.ErrInvalidFileRange
	case requested == 0:
		return workspaceapp.DefaultFileReadBytes, nil
	default:
		return min(requested, workspaceapp.MaxFileReadBytes), nil
	}
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
