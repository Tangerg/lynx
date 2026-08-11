// Package git exposes workspace version-control reads through the Git binary.
// It returns transport-neutral values and classifies an unavailable binary and
// a non-repository directory separately from a clean repository.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// ErrUnavailable means the git binary isn't on PATH. ErrNotRepo means the
// directory (or its ancestors) isn't a git work tree.
var (
	ErrUnavailable = errors.New("git: binary not available")
	ErrNotRepo     = errors.New("git: not a repository")
)

// Status is Git's normalized working-tree state for one file.
type Status string

const (
	StatusAdded     Status = "added"
	StatusModified  Status = "modified"
	StatusDeleted   Status = "deleted"
	StatusRenamed   Status = "renamed"
	StatusUntracked Status = "untracked"
)

// FileChange is one entry of a working-tree scan. Added/Removed are the line
// deltas; they are meaningless for a Binary file (the caller omits them when
// Binary is true rather than reporting a fake 0).
type FileChange struct {
	Path         string
	Status       Status
	PreviousPath string // set only for renames
	Added        int
	Removed      int
	Binary       bool
}

var availableOnce struct {
	sync.Once
	ok bool
}

// Available reports whether the git binary is on PATH. Cached after the first
// probe (PATH doesn't change mid-process).
func Available() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("git")
		availableOnce.ok = err == nil
	})
	return availableOnce.ok
}

// IsRepo reports whether dir is inside a git work tree. False when git is
// unavailable.
func IsRepo(ctx context.Context, dir string) bool {
	if !Available() {
		return false
	}
	out, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// run executes `git -C dir <args...>` with hooks disabled and returns stdout.
// A non-zero exit is returned as an error carrying stderr.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runAllowingExitCode(ctx, dir, -1, args...)
}

// runAllowingExitCode executes Git like run and additionally treats one
// documented nonzero status as success. Git uses status 1 for read-only
// predicates and for `diff --no-index` when differences exist.
func runAllowingExitCode(ctx context.Context, dir string, allowedExitCode int, args ...string) (string, error) {
	// Workspace VCS operations are observations. Suppress Git's optional index
	// refreshes so commands such as status do not take index.lock merely to
	// improve a later read. Some Git commands still perform mandatory metadata
	// refreshes; the workspace watcher compares semantic Git state before it
	// publishes and therefore does not expose those implementation writes.
	full := append([]string{"--no-optional-locks", "-C", dir, "-c", "core.quotepath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ctx.Err() == nil && errors.As(err, &exitErr) && exitErr.ExitCode() == allowedExitCode {
			return stdout.String(), nil
		}
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrUnavailable
		}
		command := strings.Join(args, " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("git %s: %s: %w", command, msg, err)
		}
		return stdout.String(), fmt.Errorf("git %s: %w", command, err)
	}
	return stdout.String(), nil
}

// ListChanges scans the working tree against HEAD: tracked changes (with line
// counts + rename detection) plus untracked files. Returns ErrNotRepo when dir
// isn't a repo. Result order is git's (roughly path order).
func ListChanges(ctx context.Context, dir string) ([]FileChange, error) {
	if !Available() {
		return nil, ErrUnavailable
	}
	if !IsRepo(ctx, dir) {
		return nil, ErrNotRepo
	}

	// status --porcelain gives the status letter + path (+ rename source);
	// -z NUL-delimits so paths with spaces/newlines stay intact.
	statusOut, err := run(ctx, dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	changes, order := parseStatusZ(statusOut)

	// numstat gives added/removed (+ binary "-\t-") for tracked changes vs
	// HEAD, with rename detection (-M). Untracked files aren't in HEAD, so
	// they won't appear here — their counts are filled below.
	head, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(head) != "" {
		numOut, err := run(ctx, dir, "diff", "--numstat", "-M", "-z", "HEAD")
		if err != nil {
			return nil, err
		}
		if err := applyNumstatZ(numOut, changes); err != nil {
			return nil, err
		}
	}

	out := make([]FileChange, 0, len(order))
	for _, p := range order {
		out = append(out, *changes[p])
	}
	return out, nil
}

// ListFiles lists the non-ignored files under dir, honoring .gitignore: tracked
// files (--cached) plus untracked-but-not-ignored ones (--others
// --exclude-standard). Optionally scoped to relPath (a pathspec relative to
// dir). Paths are relative to dir, recursive, slash-separated; -z keeps odd
// names (spaces/newlines) intact. Returns ErrNotRepo when dir isn't a repo, so
// the caller can fall back to a plain filesystem walk.
func ListFiles(ctx context.Context, dir, relPath string) ([]string, error) {
	if !Available() {
		return nil, ErrUnavailable
	}
	if !IsRepo(ctx, dir) {
		return nil, ErrNotRepo
	}
	args := []string{"ls-files", "--cached", "--others", "--exclude-standard", "-z"}
	if relPath != "" && relPath != "." {
		args = append(args, "--", relPath)
	}
	out, err := run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}

// parseStatusZ parses `git status --porcelain=v1 -z`. Each record is "XY path"
// (NUL-terminated); a rename adds a second NUL-terminated field (the original
// path). Returns a path→change map plus the encounter order.
func parseStatusZ(out string) (map[string]*FileChange, []string) {
	changes := map[string]*FileChange{}
	var order []string
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if len(rec) < 3 {
			continue
		}
		xy, path := rec[:2], rec[3:]
		fc := &FileChange{Path: path, Status: statusFromXY(xy)}
		if xy[0] == 'R' || xy[1] == 'R' {
			// rename: the next NUL field is the original path
			if i+1 < len(fields) {
				fc.PreviousPath = fields[i+1]
				i++
			}
			fc.Status = StatusRenamed
		}
		changes[path] = fc
		order = append(order, path)
	}
	return changes, order
}

// statusFromXY maps a porcelain XY code to a Status. Untracked is "??"; a
// deletion in either column is deleted; an addition is added; otherwise
// modified. (Rename is handled by the caller, which sees the R code.)
func statusFromXY(xy string) Status {
	switch {
	case xy == "??":
		return StatusUntracked
	case xy[0] == 'A' || xy[1] == 'A':
		return StatusAdded
	case xy[0] == 'D' || xy[1] == 'D':
		return StatusDeleted
	default:
		return StatusModified
	}
}

// applyNumstatZ folds `git diff --numstat -z` output into the change map.
// Each record is "added\tremoved\tpath"; a binary file reports "-\t-". With
// -z, a rename emits the path as two extra NUL fields (old, new) after the
// counts line instead of inline.
func applyNumstatZ(out string, changes map[string]*FileChange) error {
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		line := fields[i]
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			return fmt.Errorf("git: malformed numstat record %q", line)
		}
		addS, remS, path := parts[0], parts[1], parts[2]
		if path == "" {
			// rename under -z: path is empty here; old+new follow as NUL fields
			if i+2 < len(fields) {
				path = fields[i+2] // new path
				i += 2
			} else {
				return fmt.Errorf("git: malformed rename numstat record %q", line)
			}
		}
		if addS == "-" || remS == "-" {
			if fc := changes[path]; fc != nil {
				fc.Binary = true
			}
			continue
		}
		added, err := strconv.Atoi(addS)
		if err != nil || added < 0 {
			return fmt.Errorf("git: invalid added-line count %q for %q", addS, path)
		}
		removed, err := strconv.Atoi(remS)
		if err != nil || removed < 0 {
			return fmt.Errorf("git: invalid removed-line count %q for %q", remS, path)
		}
		fc := changes[path]
		if fc == nil {
			continue
		}
		fc.Added = added
		fc.Removed = removed
	}
	return nil
}
