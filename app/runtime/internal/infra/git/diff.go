package git

import (
	"context"
	"errors"
	"os"
	"strings"
)

// ErrNoBase means mode=base couldn't resolve a base branch (no remote /
// origin/HEAD, no main/master, detached HEAD). The caller maps it to
// invalid_params — NOT vcs_unavailable (that's "not a repo"), per AUX_API §2.3.
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

// Row is one structured unified-diff row. Type ∈ hunk|context|added|deleted
// (matches the wire DiffRow). Code is the line content (without the +/-/space).
type Row struct {
	Type      string
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

// Diff returns the per-file parsed diff for dir under the given mode, optionally
// scoped to relPath (relative to dir). Worktree mode appends untracked files as
// all-added diffs. Returns ErrNotRepo / ErrUnavailable / ErrNoBase as the
// caller needs to distinguish them.
func Diff(ctx context.Context, dir, relPath string, mode Mode) ([]DiffFile, error) {
	patch, untracked, err := diffSources(ctx, dir, relPath, mode)
	if err != nil {
		return nil, err
	}
	files := parseUnifiedDiff(patch)
	for _, u := range untracked {
		if df, ok := untrackedDiffFile(dir, u); ok {
			files = append(files, df)
		}
	}
	return files, nil
}

// RawDiff returns the raw unified patch text (format=raw). Worktree mode appends
// per-untracked no-index patches so the raw view matches the parsed one.
func RawDiff(ctx context.Context, dir, relPath string, mode Mode) (string, error) {
	patch, untracked, err := diffSources(ctx, dir, relPath, mode)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(patch)
	for _, u := range untracked {
		// no-index diff of /dev/null vs the file; exit code 1 (differences) is
		// normal, so run() returning an error here is tolerated for non-empty out.
		out, _ := run(ctx, dir, "diff", "--no-index", "--", os.DevNull, u)
		b.WriteString(out)
	}
	return b.String(), nil
}
