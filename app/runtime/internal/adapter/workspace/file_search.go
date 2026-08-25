package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/textread"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

// Grep searches the same finite, ignore-aware file catalog exposed by the
// workspace browser. It scans in process so source, line, retained material,
// cancellation, and exact-total semantics remain owned by this product port
// instead of inheriting a subprocess executor's post-hoc result slicing.
func (FileBrowser) Grep(ctx context.Context, root string, input workspaceapp.GrepPlan) (workspaceapp.GrepResult, error) {
	if cause := context.Cause(ctx); cause != nil {
		return workspaceapp.GrepResult{}, cause
	}
	if input.Pattern == nil || input.Limit <= 0 || input.Limit > workspaceapp.MaxGrepLimit {
		return workspaceapp.GrepResult{}, workspaceapp.ErrInvalidGrepQuery
	}
	entries, err := ListFiles(ctx, root, ListFilesOptions{Path: input.Path, Recursive: true})
	if err != nil {
		if errors.Is(err, ErrListingTooLarge) {
			return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
		}
		return workspaceapp.GrepResult{}, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.GrepResult{}, fmt.Errorf("workspace: resolve search root: %w", err)
	}

	result := workspaceapp.GrepResult{Matches: []workspaceapp.GrepMatch{}}
	remainingSource := int64(workspaceapp.MaxGrepSourceBytes)
	retainedBytes := 0
	retain := true
	for _, entry := range entries {
		if entry.Kind != EntryFile || entry.SizeBytes > workspaceapp.MaxGrepFileBytes {
			continue
		}
		if cause := context.Cause(ctx); cause != nil {
			return workspaceapp.GrepResult{}, cause
		}
		path, pathErr := rootRelativeGrepPath(root, canonicalRoot, filepath.Join(root, filepath.FromSlash(entry.Path)))
		if pathErr != nil {
			return workspaceapp.GrepResult{}, pathErr
		}
		file, openErr := os.Open(filepath.Join(canonicalRoot, filepath.FromSlash(path)))
		if openErr != nil {
			return workspaceapp.GrepResult{}, fmt.Errorf("workspace: open search file %q: %w", path, openErr)
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return workspaceapp.GrepResult{}, fmt.Errorf("workspace: inspect search file %q: %w", path, statErr)
		}
		if !info.Mode().IsRegular() || info.Size() > workspaceapp.MaxGrepFileBytes {
			if closeErr := file.Close(); closeErr != nil {
				return workspaceapp.GrepResult{}, fmt.Errorf("workspace: close search file %q: %w", path, closeErr)
			}
			continue
		}
		if remainingSource <= 0 {
			_ = file.Close()
			return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
		}

		inputBytes := min(workspaceapp.MaxGrepFileBytes, remainingSource)
		counter := &searchByteCounter{reader: file}
		matchLimit, resultBytes := 0, 0
		if retain {
			matchLimit = input.Limit - len(result.Matches)
			resultBytes = workspaceapp.MaxGrepResultBytes - retainedBytes
		}
		fileResult, scanErr := grepFile(ctx, counter, path, input, inputBytes, matchLimit, resultBytes)
		closeErr := file.Close()
		if closeErr != nil {
			return workspaceapp.GrepResult{}, fmt.Errorf("workspace: close search file %q: %w", path, closeErr)
		}
		remainingSource -= counter.bytes
		if remainingSource < 0 {
			return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
		}
		if scanErr != nil {
			switch {
			case errors.Is(scanErr, textread.ErrInvalidText), errors.Is(scanErr, textread.ErrLineTooLarge):
				// Binary and pathological single-line files are not members of the
				// searchable text corpus. Discard the whole file, including any rows
				// observed before invalid material, just as a binary-aware grep does.
				continue
			case errors.Is(scanErr, textread.ErrInputTooLarge):
				return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
			default:
				return workspaceapp.GrepResult{}, fmt.Errorf("workspace: scan search file %q: %w", path, scanErr)
			}
		}
		if fileResult.total > math.MaxInt-result.Total {
			return workspaceapp.GrepResult{}, workspaceapp.ErrGrepResultTooLarge
		}
		result.Total += fileResult.total
		result.Matches = append(result.Matches, fileResult.matches...)
		retainedBytes += fileResult.materialBytes
		if fileResult.exhausted {
			retain = false
		}
	}
	return result, nil
}

type grepFileResult struct {
	matches       []workspaceapp.GrepMatch
	total         int
	materialBytes int
	exhausted     bool
}

func grepFile(
	ctx context.Context,
	reader io.Reader,
	path string,
	input workspaceapp.GrepPlan,
	inputBytes int64,
	matchLimit int,
	resultBytes int,
) (grepFileResult, error) {
	result := grepFileResult{matches: []workspaceapp.GrepMatch{}}
	err := textread.VisitLines(ctx, reader, textread.Limits{
		InputBytes: inputBytes,
		LineBytes:  workspaceapp.MaxGrepLineBytes,
	}, func(number int, line []byte) error {
		if !input.Pattern.Match(line) {
			return nil
		}
		result.total++
		if result.exhausted {
			return nil
		}
		rowBytes := len(path) + len(line)
		if len(result.matches) >= matchLimit || rowBytes > resultBytes-result.materialBytes {
			result.exhausted = true
			return nil
		}
		result.matches = append(result.matches, workspaceapp.GrepMatch{
			Path: path, LineNumber: number, Text: string(line),
		})
		result.materialBytes += rowBytes
		return nil
	})
	return result, err
}

// rootRelativeGrepPath converts a host path into the slash-separated workspace
// identity promised by workspace.files.search. Canonicalizing both sides also
// rejects a candidate that escapes through an in-root symlink.
func rootRelativeGrepPath(root, canonicalRoot, candidate string) (string, error) {
	abs := candidate
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, candidate)
	}
	canonicalCandidate, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve search candidate %q: %w", candidate, err)
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("workspace: relativize search candidate %q: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return filepath.ToSlash(rel), nil
}

type searchByteCounter struct {
	reader io.Reader
	bytes  int64
}

func (reader *searchByteCounter) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	reader.bytes += int64(read)
	return read, err
}
