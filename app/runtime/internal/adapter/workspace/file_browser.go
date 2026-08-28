package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/tools/textread"
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
