package git

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// diffSources runs the tracked-changes git diff for the mode and returns the
// patch text plus the untracked file list (worktree mode only).
func diffSources(ctx context.Context, dir, relPath string, mode Mode) (patch []byte, untracked []string, err error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, nil, err
	}
	if !repository {
		return nil, nil, ErrNotRepo
	}

	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "-M", "--relative"}
	switch mode {
	case Base:
		base, berr := mergeBase(ctx, dir)
		if berr != nil {
			return nil, nil, berr
		}
		args = append(args, base)
	default: // Worktree
		head, headErr := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
		if headErr != nil {
			return nil, nil, headErr
		}
		if len(bytes.TrimSpace(head)) == 0 {
			untracked, err = untrackedPaths(ctx, dir, relPath)
			return nil, untracked, err
		}
		args = append(args, "HEAD")
	}
	scopePath, err := gitPathRelativeToWorkspace(dir, relPath)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, "--", scopePath)
	patch, err = run(ctx, dir, args...)
	if err != nil {
		return nil, nil, err
	}

	if mode == Worktree {
		untracked, err = untrackedPaths(ctx, dir, relPath)
		if err != nil {
			return nil, nil, err
		}
	}
	return patch, untracked, nil
}

func gitPathRelativeToWorkspace(dir, path string) (string, error) {
	if path == "" {
		return ".", nil
	}
	if !filepath.IsAbs(path) {
		relative := filepath.Clean(path)
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("git: path %q is outside workspace %q", path, dir)
		}
		return filepath.ToSlash(relative), nil
	}
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return "", fmt.Errorf("git: resolve workspace-relative path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git: path %q is outside workspace %q", path, dir)
	}
	return filepath.ToSlash(relative), nil
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
	return strings.TrimSpace(string(out)), nil
}

// defaultBranch resolves the base branch: origin/HEAD → main → master.
func defaultBranch(ctx context.Context, dir string) (string, error) {
	if out, err := run(ctx, dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(string(out)) // refs/remotes/origin/main
		if b, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
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
