package git

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// untrackedPaths lists untracked files (status ??), optionally under relPath.
func untrackedPaths(ctx context.Context, dir, relPath string) ([]string, error) {
	scopePath, err := gitPathRelativeToWorkspace(dir, relPath)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z", "--", scopePath)
	if err != nil {
		return nil, err
	}
	var paths []string
	for path := range strings.SplitSeq(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
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
	df.Rows = append(df.Rows, Row{Type: RowHunk, Text: "@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@"})
	for i, ln := range lines {
		df.Rows = append(df.Rows, Row{Type: RowAdded, RightLine: i + 1, Code: ln})
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
