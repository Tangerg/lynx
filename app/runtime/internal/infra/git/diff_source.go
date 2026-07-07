package git

import (
	"context"
	"strings"
)

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
