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

	"github.com/Tangerg/lynx/app/runtime/internal/infra/gitprocess"
)

// ErrUnavailable means the git binary isn't on PATH. ErrNotRepo means the
// directory (or its ancestors) isn't a git work tree.
var (
	ErrUnavailable    = errors.New("git: binary not available")
	ErrNotRepo        = errors.New("git: not a repository")
	ErrResultTooLarge = errors.New("git: result too large")
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

// IsRepo reports whether dir is inside a Git work tree. An unavailable binary
// and request cancellation remain errors so callers can deliberately choose
// fallback or termination instead of conflating both with a non-repository.
func IsRepo(ctx context.Context, dir string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("git rev-parse --is-inside-work-tree: %w", err)
	}
	if !Available() {
		return false, ErrUnavailable
	}
	args := []string{"rev-parse", "--is-inside-work-tree"}
	full := append([]string{"--no-pager", "--no-optional-locks", "-C", dir, "-c", "core.quotepath=false"}, args...)
	// The only expected negative result is identified from Git's stable English
	// diagnostic. Exit 128 alone is not enough: unsafe ownership, corrupt
	// metadata, and an unreadable repository use the same status and must remain
	// observable to the caller.
	result, err := gitprocess.Run(ctx, []string{"LC_ALL=C", "LANG=C"}, full...)
	if err != nil {
		return false, gitProcessError(args, err)
	}
	if result.ExitCode != 0 {
		diagnostic := strings.TrimSpace(result.Stderr)
		if result.ExitCode == 128 && isNotRepositoryDiagnostic(diagnostic) {
			return false, nil
		}
		return false, gitExitError(args, result.ExitCode, diagnostic)
	}
	return bytes.Equal(bytes.TrimSpace(result.Stdout), []byte("true")), nil
}

func isNotRepositoryDiagnostic(diagnostic string) bool {
	return strings.HasPrefix(diagnostic, "fatal: not a git repository")
}

// run executes `git -C dir <args...>` without a pager, optional locks, or
// inherited Git control variables and returns stdout. A non-zero exit is
// returned as an error carrying bounded stderr.
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return runAllowingExitCode(ctx, dir, -1, args...)
}

// runAllowingExitCode executes Git like run and additionally treats one
// documented nonzero status as success. Git uses status 1 for read-only
// predicates and for `diff --no-index` when differences exist.
func runAllowingExitCode(ctx context.Context, dir string, allowedExitCode int, args ...string) ([]byte, error) {
	// Workspace VCS operations are observations. Suppress Git's optional index
	// refreshes so commands such as status do not take index.lock merely to
	// improve a later read. Some Git commands still perform mandatory metadata
	// refreshes; the workspace watcher compares semantic Git state before it
	// publishes and therefore does not expose those implementation writes.
	full := append([]string{"--no-pager", "--no-optional-locks", "-C", dir, "-c", "core.quotepath=false"}, args...)
	result, err := gitprocess.Run(ctx, []string{"LC_ALL=C", "LANG=C"}, full...)
	if err != nil {
		return nil, gitProcessError(args, err)
	}
	if result.ExitCode == 0 || result.ExitCode == allowedExitCode {
		return result.Stdout, nil
	}
	return result.Stdout, gitExitError(args, result.ExitCode, strings.TrimSpace(result.Stderr))
}

func gitProcessError(args []string, err error) error {
	command := strings.Join(args, " ")
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("git %s: %w", command, err)
	case errors.Is(err, exec.ErrNotFound):
		return ErrUnavailable
	case errors.Is(err, gitprocess.ErrOutputTooLarge):
		return fmt.Errorf("%w: git %s output exceeds 64 MiB", ErrResultTooLarge, command)
	default:
		return fmt.Errorf("git %s: %w", command, err)
	}
}

func gitExitError(args []string, exitCode int, diagnostic string) error {
	command := strings.Join(args, " ")
	if diagnostic != "" {
		return fmt.Errorf("git %s: %s: exit code %d", command, diagnostic, exitCode)
	}
	return fmt.Errorf("git %s: exit code %d", command, exitCode)
}

// ListChanges scans the working tree against HEAD: tracked changes (with line
// counts + rename detection) plus untracked files. It rejects the complete
// catalog before retaining more than maxChanges entries. Returns ErrNotRepo
// when dir isn't a repo. Result order is Git's (roughly path order).
func ListChanges(ctx context.Context, dir string, maxChanges int) ([]FileChange, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !repository {
		return nil, ErrNotRepo
	}
	prefixOut, err := run(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	repositoryPrefix := strings.TrimRight(string(prefixOut), "\r\n")

	// status --porcelain gives the status letter + path (+ rename source);
	// -z NUL-delimits so paths with spaces/newlines stay intact. The explicit
	// pathspec keeps a nested WorkspaceRef jailed to its own resource root.
	statusOut, err := run(ctx, dir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	changes, order, err := parseStatusZ(statusOut, maxChanges)
	if err != nil {
		return nil, err
	}

	// numstat gives added/removed (+ binary "-\t-") for tracked changes vs
	// HEAD, with rename detection (-M). Untracked files aren't in HEAD, so
	// they won't appear here — their counts are filled below.
	head, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(head)) != 0 {
		numOut, err := run(ctx, dir, "diff", "--numstat", "--no-ext-diff", "--no-textconv", "-M", "-z", "HEAD", "--", ".")
		if err != nil {
			return nil, err
		}
		if err := applyNumstatZ(numOut, changes); err != nil {
			return nil, err
		}
	}

	out := make([]FileChange, 0, len(order))
	for _, p := range order {
		change := *changes[p]
		path, pathInside := workspaceRelativeGitPath(change.Path, repositoryPrefix)
		if change.Status == StatusRenamed {
			previousPath, previousInside := workspaceRelativeGitPath(change.PreviousPath, repositoryPrefix)
			switch {
			case pathInside && previousInside:
				change.Path, change.PreviousPath = path, previousPath
			case pathInside:
				change.Path, change.PreviousPath, change.Status = path, "", StatusAdded
			case previousInside:
				change.Path, change.PreviousPath, change.Status = previousPath, "", StatusDeleted
			default:
				continue
			}
		} else {
			if !pathInside {
				continue
			}
			change.Path = path
		}
		if change.Status == StatusUntracked {
			added, binary, err := untrackedFileStat(ctx, dir, change.Path)
			if err != nil {
				return nil, err
			}
			change.Added, change.Removed, change.Binary = added, 0, binary
		}
		out = append(out, change)
	}
	return out, nil
}

// workspaceRelativeGitPath maps Git's repository-root-relative porcelain path
// back into the WorkspaceRef root. Porcelain v1 with -z deliberately ignores
// status.relativePaths, so this translation is part of the adapter boundary.
func workspaceRelativeGitPath(path, repositoryPrefix string) (string, bool) {
	if path == "" {
		return "", false
	}
	if repositoryPrefix == "" {
		return path, true
	}
	relative, ok := strings.CutPrefix(path, repositoryPrefix)
	return relative, ok && relative != ""
}

// ListFiles lists the non-ignored files under dir, honoring .gitignore: tracked
// files (--cached) plus untracked-but-not-ignored ones (--others
// --exclude-standard). Optionally scoped to relPath (a pathspec relative to
// dir). Paths are relative to dir, recursive, slash-separated; -z keeps odd
// names (spaces/newlines) intact. Returns ErrNotRepo when dir isn't a repo, so
// the caller can fall back to a plain filesystem walk.
func ListFiles(ctx context.Context, dir, relPath string, maxFiles int) ([]string, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !repository {
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
	for path := range bytes.SplitSeq(out, []byte{0}) {
		if len(path) != 0 {
			if maxFiles <= 0 || len(files) >= maxFiles {
				return nil, fmt.Errorf("%w: more than %d workspace files", ErrResultTooLarge, maxFiles)
			}
			files = append(files, string(path))
		}
	}
	return files, nil
}

// parseStatusZ parses `git status --porcelain=v1 -z`. Each record is "XY path"
// (NUL-terminated); a rename adds a second NUL-terminated field (the original
// path). Returns a path→change map plus the encounter order.
func parseStatusZ(out []byte, maxChanges int) (map[string]*FileChange, []string, error) {
	changes := map[string]*FileChange{}
	var order []string
	next := func() ([]byte, bool) {
		if len(out) == 0 {
			return nil, false
		}
		index := bytes.IndexByte(out, 0)
		if index < 0 {
			field := out
			out = nil
			return field, true
		}
		field := out[:index]
		out = out[index+1:]
		return field, true
	}
	for {
		rec, ok := next()
		if !ok {
			break
		}
		if len(rec) < 3 {
			continue
		}
		if maxChanges <= 0 || len(order) >= maxChanges {
			return nil, nil, fmt.Errorf("%w: more than %d workspace changes", ErrResultTooLarge, maxChanges)
		}
		xy, path := string(rec[:2]), string(rec[3:])
		fc := &FileChange{Path: path, Status: statusFromXY(xy)}
		if xy[0] == 'R' || xy[1] == 'R' {
			// rename: the next NUL field is the original path
			if previous, ok := next(); ok {
				fc.PreviousPath = string(previous)
			}
			fc.Status = StatusRenamed
		}
		changes[path] = fc
		order = append(order, path)
	}
	return changes, order, nil
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
func applyNumstatZ(out []byte, changes map[string]*FileChange) error {
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		line := fields[i]
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte{'\t'}, 3)
		if len(parts) < 3 {
			return fmt.Errorf("git: malformed numstat record %q", line)
		}
		addS, remS, path := string(parts[0]), string(parts[1]), string(parts[2])
		if path == "" {
			// rename under -z: path is empty here; old+new follow as NUL fields
			if i+2 < len(fields) {
				path = string(fields[i+2]) // new path
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
