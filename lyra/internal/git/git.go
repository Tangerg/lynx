// Package git is a thin exec-git capability layer for the workspace VCS
// surface (workspace.listFileChanges / getDiff). It shells out to the git
// binary — no embedded git library — and is transport-neutral: it returns
// plain structs, never rpc/protocol types, so the protocol server maps them
// to the wire. Missing git binary or a non-repo directory surface as typed
// errors (ErrUnavailable / ErrNotRepo) rather than empty results, so the
// caller can keep "no git" / "not a repo" / "clean repo" distinct.
package git

import (
	"bytes"
	"context"
	"errors"
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

// Status is the working-tree state of one file (matches the wire vocabulary).
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
	full := append([]string{"-C", dir, "-c", "core.quotepath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrUnavailable
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New("git: " + msg)
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
	numOut, err := run(ctx, dir, "diff", "--numstat", "-M", "-z", "HEAD")
	if err == nil { // a fresh repo with no HEAD yields an error; counts stay 0
		applyNumstatZ(numOut, changes)
	}

	out := make([]FileChange, 0, len(order))
	for _, p := range order {
		out = append(out, *changes[p])
	}
	return out, nil
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
func applyNumstatZ(out string, changes map[string]*FileChange) {
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		line := fields[i]
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		addS, remS, path := parts[0], parts[1], parts[2]
		if path == "" {
			// rename under -z: path is empty here; old+new follow as NUL fields
			if i+2 < len(fields) {
				path = fields[i+2] // new path
				i += 2
			}
		}
		fc := changes[path]
		if fc == nil {
			continue
		}
		if addS == "-" || remS == "-" {
			fc.Binary = true
			continue
		}
		fc.Added, _ = strconv.Atoi(addS)
		fc.Removed, _ = strconv.Atoi(remS)
	}
}
