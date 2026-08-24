package git

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ErrNoBase means mode=base could not resolve a base branch: there is no remote
// default, main/master branch, or attached HEAD. It is distinct from a missing
// repository or unavailable Git binary.
var ErrNoBase = errors.New("git: cannot resolve base branch")

// Mode selects what getDiff compares against.
type Mode string

const (
	// Worktree = working-tree changes vs HEAD, INCLUDING untracked files.
	Worktree Mode = "worktree"
	// Base = changes vs the merge-base with the default branch (committed +
	// working-tree tracked changes; untracked excluded).
	Base Mode = "base"
)

// RowType is the parser-owned classification of a unified-diff row.
type RowType string

const (
	RowHunk    RowType = "hunk"
	RowContext RowType = "context"
	RowAdded   RowType = "added"
	RowDeleted RowType = "deleted"
)

// Row is one structured unified-diff row. Code is the line content without the
// leading addition, deletion, or context marker.
type Row struct {
	Type      RowType
	Text      string // hunk header text (Type=hunk)
	LeftLine  int
	RightLine int
	Code      string
}

// DiffFile is one file's parsed diff. Added/Removed are counted from the rows;
// they are meaningless for a Binary file (Rows empty, caller omits the counts).
type DiffFile struct {
	Path         string
	Status       Status
	PreviousPath string
	Added        int
	Removed      int
	Binary       bool
	Rows         []Row
}

// Diff returns a whole-file parsed projection for dir under the given mode,
// optionally scoped to relPath (relative to dir). Worktree mode includes
// untracked files through Git's no-index patch semantics. maxFiles, maxRows,
// and maxBytes are hard pre-materialization boundaries; truncated reports a
// complete-file cut. Repository and process sentinels remain distinguishable.
func Diff(ctx context.Context, dir, relPath string, mode Mode, maxFiles, maxRows, maxBytes int) ([]DiffFile, bool, error) {
	patch, untracked, err := completeDiff(ctx, dir, relPath, mode, maxBytes)
	if err != nil {
		return nil, false, err
	}
	files, truncated, err := parseUnifiedDiff(patch, maxFiles, maxRows)
	if err != nil {
		return nil, false, err
	}
	for index := range files {
		if _, ok := untracked[files[index].Path]; ok {
			files[index].Status = StatusUntracked
		}
	}
	return files, truncated, nil
}

// RawDiff returns at most maxBytes of complete raw unified patch text. Worktree
// mode appends per-untracked no-index patches so the raw view matches the
// parsed one; an oversized aggregate fails instead of returning a partial patch.
func RawDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) (string, error) {
	patch, _, err := completeDiff(ctx, dir, relPath, mode, maxBytes)
	if err != nil {
		return "", err
	}
	return string(patch), nil
}

func completeDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) ([]byte, map[string]struct{}, error) {
	if maxBytes <= 0 {
		return nil, nil, fmt.Errorf("%w: diff requires a positive byte limit", ErrResultTooLarge)
	}
	patch, untrackedPaths, err := diffSources(ctx, dir, relPath, mode)
	if err != nil {
		return nil, nil, err
	}
	if len(patch) > maxBytes {
		return nil, nil, fmt.Errorf("%w: aggregate diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	untracked := make(map[string]struct{}, len(untrackedPaths))
	for _, path := range untrackedPaths {
		untracked[path] = struct{}{}
		// no-index diff of /dev/null vs the file; Git uses exit code 1 to report
		// the expected fact that the two paths differ.
		out, err := runAllowingExitCode(
			ctx,
			dir,
			1,
			"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", os.DevNull, path,
		)
		if err != nil {
			return nil, nil, err
		}
		patch, err = appendDiffPatch(patch, out, maxBytes)
		if err != nil {
			return nil, nil, err
		}
	}
	return patch, untracked, nil
}

func appendDiffPatch(existing, addition []byte, maxBytes int) ([]byte, error) {
	if len(addition) == 0 {
		return existing, nil
	}
	separator := 0
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = 1
	}
	if len(existing) > maxBytes-separator || len(addition) > maxBytes-len(existing)-separator {
		return nil, fmt.Errorf("%w: aggregate diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	if separator != 0 {
		existing = append(existing, '\n')
	}
	return append(existing, addition...), nil
}
