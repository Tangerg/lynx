package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxUntrackedDiffPaths = 10_000

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
	for path := range bytes.SplitSeq(out, []byte{0}) {
		if len(path) != 0 {
			if len(paths) >= maxUntrackedDiffPaths {
				return nil, fmt.Errorf("%w: more than %d untracked diff paths", ErrResultTooLarge, maxUntrackedDiffPaths)
			}
			paths = append(paths, string(path))
		}
	}
	return paths, nil
}

// untrackedFileStat streams one untracked regular file for its Git-facing line
// count and binary bit. A symbolic link is one link-target line; its referent is
// never read as workspace material.
func untrackedFileStat(ctx context.Context, dir, rel string) (int, bool, error) {
	path := filepath.Join(dir, rel)
	info, err := os.Lstat(path)
	if err != nil {
		return 0, false, fmt.Errorf("git: inspect untracked file %q: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 1, false, nil
	}
	if !info.Mode().IsRegular() {
		return 0, true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, false, fmt.Errorf("git: open untracked file %q: %w", rel, err)
	}
	defer file.Close()
	buffer := make([]byte, 64<<10)
	lines := 0
	var last byte
	nonempty := false
	for {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			nonempty = true
			chunk := buffer[:count]
			if bytes.IndexByte(chunk, 0) >= 0 {
				return 0, true, nil
			}
			lines += bytes.Count(chunk, []byte{'\n'})
			last = chunk[len(chunk)-1]
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return 0, false, fmt.Errorf("git: read untracked file %q: %w", rel, readErr)
		}
	}
	if nonempty && last != '\n' {
		lines++
	}
	return lines, false, nil
}
