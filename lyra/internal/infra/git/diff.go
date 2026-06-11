package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// diffSources runs the tracked-changes git diff for the mode and returns the
// patch text plus the untracked file list (worktree mode only).
func diffSources(ctx context.Context, dir, relPath string, mode Mode) (patch string, untracked []string, err error) {
	if !Available() {
		return "", nil, ErrUnavailable
	}
	if !IsRepo(ctx, dir) {
		return "", nil, ErrNotRepo
	}

	args := []string{"diff", "-M"}
	switch mode {
	case Base:
		base, berr := mergeBase(ctx, dir)
		if berr != nil {
			return "", nil, berr
		}
		args = append(args, base)
	default: // Worktree
		args = append(args, "HEAD")
	}
	if relPath != "" {
		args = append(args, "--", relPath)
	}
	patch, err = run(ctx, dir, args...)
	if err != nil {
		// Fresh repo (no HEAD) yields an error; treat as empty diff rather than fail.
		patch = ""
	}

	if mode == Worktree {
		untracked = untrackedPaths(ctx, dir, relPath)
	}
	return patch, untracked, nil
}

// untrackedPaths lists untracked files (status ??), optionally under relPath.
func untrackedPaths(ctx context.Context, dir, relPath string) []string {
	out, err := run(ctx, dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil
	}
	var paths []string
	for rec := range strings.SplitSeq(out, "\x00") {
		if len(rec) < 3 || rec[:2] != "??" {
			continue
		}
		p := rec[3:]
		if relPath != "" && p != relPath && !strings.HasPrefix(p, relPath+"/") {
			continue
		}
		paths = append(paths, p)
	}
	return paths
}

// untrackedDiffFile builds an all-added DiffFile by reading the untracked file.
// Binary files surface as binary:true with no rows.
func untrackedDiffFile(dir, rel string) (DiffFile, bool) {
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return DiffFile{}, false
	}
	df := DiffFile{Path: rel, Status: StatusUntracked}
	if looksBinary(data) {
		df.Binary = true
		return df, true
	}
	text := strings.TrimSuffix(string(data), "\n")
	lines := strings.Split(text, "\n")
	if len(data) == 0 {
		lines = nil
	}
	df.Rows = append(df.Rows, Row{Type: "hunk", Text: "@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@"})
	for i, ln := range lines {
		df.Rows = append(df.Rows, Row{Type: "added", RightLine: i + 1, Code: ln})
	}
	df.Added = len(lines)
	return df, true
}

// looksBinary reports whether data appears to be binary (a NUL byte in the
// first 8KB — git's own heuristic).
func looksBinary(data []byte) bool {
	n := min(len(data), 8000)
	for i := range n {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// mergeBase resolves the merge-base of HEAD with the default branch.
func mergeBase(ctx context.Context, dir string) (string, error) {
	branch, err := defaultBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	out, err := run(ctx, dir, "merge-base", "HEAD", branch)
	if err != nil {
		return "", ErrNoBase
	}
	return strings.TrimSpace(out), nil
}

// defaultBranch resolves the base branch: origin/HEAD → main → master.
func defaultBranch(ctx context.Context, dir string) (string, error) {
	if out, err := run(ctx, dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out) // refs/remotes/origin/main
		if b := strings.TrimPrefix(ref, "refs/remotes/"); b != ref {
			return b, nil
		}
	}
	for _, b := range []string{"main", "master"} {
		if _, err := run(ctx, dir, "rev-parse", "--verify", "--quiet", b); err == nil {
			return b, nil
		}
	}
	return "", ErrNoBase
}

// parseUnifiedDiff parses a `git diff` unified patch into per-file DiffFiles.
// Path comes from the +++ (new) / --- (old, for deletes) headers — one path per
// line, so unambiguous even with spaces; status from the extended headers
// (new file / deleted file / rename); added/removed counted from the rows.
func parseUnifiedDiff(patch string) []DiffFile {
	var files []DiffFile
	var cur *DiffFile
	var leftLine, rightLine int

	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, DiffFile{Status: StatusModified})
			cur = &files[len(files)-1]
		case cur == nil:
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = StatusAdded
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = StatusDeleted
		case strings.HasPrefix(line, "rename from "):
			cur.PreviousPath = strings.TrimPrefix(line, "rename from ")
			cur.Status = StatusRenamed
		case strings.HasPrefix(line, "rename to "):
			cur.Path = strings.TrimPrefix(line, "rename to ")
			cur.Status = StatusRenamed
		case strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); cur.Path == "" && p != "/dev/null" {
				cur.Path = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				cur.Path = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			leftLine, rightLine = parseHunkHeader(line)
			cur.Rows = append(cur.Rows, Row{Type: "hunk", Text: line})
		case strings.HasPrefix(line, "+"):
			cur.Rows = append(cur.Rows, Row{Type: "added", RightLine: rightLine, Code: line[1:]})
			rightLine++
			cur.Added++
		case strings.HasPrefix(line, "-"):
			cur.Rows = append(cur.Rows, Row{Type: "deleted", LeftLine: leftLine, Code: line[1:]})
			leftLine++
			cur.Removed++
		case strings.HasPrefix(line, " "):
			cur.Rows = append(cur.Rows, Row{Type: "context", LeftLine: leftLine, RightLine: rightLine, Code: line[1:]})
			leftLine++
			rightLine++
		}
	}
	return files
}

// parseHunkHeader pulls the left/right start lines out of "@@ -L,S +R,S @@ …".
func parseHunkHeader(h string) (left, right int) {
	for f := range strings.FieldsSeq(h) {
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case '-':
			left = atoiBeforeComma(f[1:])
		case '+':
			right = atoiBeforeComma(f[1:])
		}
	}
	return left, right
}

func atoiBeforeComma(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}
