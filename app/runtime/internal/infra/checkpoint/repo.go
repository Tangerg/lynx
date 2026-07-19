package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// commonExcludes keep a checkpoint from ballooning into dependency / build
// output in a repo that lacks its own .gitignore. A repo WITH a .gitignore is
// already honored by git; this is only a backstop so a no-ignore project
// doesn't snapshot node_modules on every run.
const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

// ensureRepo lazily initializes the session's shadow repo (idempotent).
func (s *Store) ensureRepo(ctx context.Context, sessionID, cwd string) (string, error) {
	gitDir := s.gitDir(sessionID)
	if repoExists(gitDir) {
		// A repository with a commit has completed at least one snapshot. An
		// initialized repository without one may be residue from an interrupted
		// first snapshot; rebuild it instead of trusting a possibly partial index
		// or alternates file.
		hasHead, err := s.hasHead(ctx, gitDir)
		if err != nil {
			return "", err
		}
		if hasHead {
			return gitDir, nil
		}
	}

	parent := filepath.Dir(gitDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("checkpoint: create repository parent: %w", err)
	}
	stagingDir, err := os.MkdirTemp(parent, "."+filepath.Base(gitDir)+".init-")
	if err != nil {
		return "", fmt.Errorf("checkpoint: create repository staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if _, err := s.git(ctx, stagingDir, cwd, "init", "-q"); err != nil {
		return "", err
	}
	if err := s.seedFrom(ctx, stagingDir, cwd); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "info", "exclude"), []byte(commonExcludes), 0o644); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	if err := publishRepo(stagingDir, gitDir); err != nil {
		return "", err
	}
	published = true
	return gitDir, nil
}

// publishRepo makes a fully initialized repository visible in one rename. The
// staging directory is a sibling of dst, so the rename cannot cross filesystems.
// An initialized repository without a commit is safe to replace: it has never
// represented a completed checkpoint boundary.
func publishRepo(stagingDir, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("checkpoint: remove incomplete repository: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: inspect repository destination: %w", err)
	}
	if err := os.Rename(stagingDir, dst); err != nil {
		return fmt.Errorf("checkpoint: publish repository: %w", err)
	}
	return nil
}

// seedFrom wires a freshly-initialized shadow repo to reuse the real repo's
// object store and index, so the first snapshot doesn't re-store the whole tree.
// Sharing objects/info/alternates lets `git add` resolve every unchanged blob
// through the real .git instead of writing a copy; seeding the index reuses the
// existing hashes instead of re-hashing every file — the cost that becomes
// significant on large checkouts.
//
// If cwd isn't a git repo, the shadow starts empty and the first `git add` does
// the full work. Once a source repo has been discovered, however, the seed is
// all-or-nothing: publishing an index without every configured object store
// would create a repository that cannot resolve unchanged files.
func (s *Store) seedFrom(ctx context.Context, gitDir, cwd string) error {
	srcObjects, err := gitIn(ctx, cwd, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil || srcObjects == "" {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("checkpoint: discover source repository: %w", ctxErr)
		}
		return nil // not a git repo we can seed from
	}

	info, err := os.Stat(srcObjects)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checkpoint: inspect source object store: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("checkpoint: source object store %q is not a directory", srcObjects)
	}

	// Share the real object DB plus any store it already borrows, keeping only
	// stores that still exist so the chain resolves. Git interprets relative
	// entries relative to the object database that owns the alternates file.
	alternates := []string{srcObjects}
	if data, err := os.ReadFile(filepath.Join(srcObjects, "info", "alternates")); err == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			p := strings.TrimSpace(line)
			if p == "" {
				continue
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(srcObjects, p)
			}
			p = filepath.Clean(p)
			if info, statErr := os.Stat(p); statErr == nil && info.IsDir() {
				alternates = append(alternates, p)
			} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("checkpoint: inspect source alternate %q: %w", p, statErr)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: read source alternates: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects", "info"), 0o755); err != nil {
		return fmt.Errorf("checkpoint: create alternates directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "objects", "info", "alternates"),
		[]byte(strings.Join(alternates, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("checkpoint: write source alternates: %w", err)
	}

	srcIndex, err := gitIn(ctx, cwd, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return fmt.Errorf("checkpoint: discover source index: %w", err)
	}
	info, err = os.Stat(srcIndex)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checkpoint: inspect source index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("checkpoint: source index %q is not a regular file", srcIndex)
	}
	if err := copyFile(srcIndex, filepath.Join(gitDir, "index")); err != nil {
		return fmt.Errorf("checkpoint: copy source index: %w", err)
	}
	return nil
}

func repoExists(gitDir string) bool {
	info, err := os.Stat(filepath.Join(gitDir, "HEAD"))
	return err == nil && info.Mode().IsRegular()
}

// materializeAlternates copies every object reachable from the shadow refs out
// of borrowed object stores, then removes the alternates link. This preserves
// the cheap source-index seed while making completed checkpoints independent of
// future pruning or deletion of the source repository.
//
// The pending file closes the crash window around verification: if a process
// stops after hiding alternates but before deleting the file, the next call
// verifies the local object graph and either completes the detach or restores
// the alternate before retrying.
func (s *Store) materializeAlternates(ctx context.Context, gitDir string) error {
	infoDir := filepath.Join(gitDir, "objects", "info")
	alternatesPath := filepath.Join(infoDir, "alternates")
	pendingPath := filepath.Join(infoDir, "alternates.pending")

	if _, err := os.Stat(pendingPath); err == nil {
		if _, err := os.Stat(alternatesPath); err == nil {
			return errors.New("checkpoint: both active and pending alternates exist")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint: inspect active alternates: %w", err)
		}
		if err := s.verifyLocalObjects(ctx, gitDir); err == nil {
			if err := os.Remove(pendingPath); err != nil {
				return fmt.Errorf("checkpoint: remove detached alternates: %w", err)
			}
			return nil
		}
		if err := os.Rename(pendingPath, alternatesPath); err != nil {
			return fmt.Errorf("checkpoint: restore interrupted alternates detach: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: inspect pending alternates: %w", err)
	}

	if _, err := os.Stat(alternatesPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checkpoint: inspect alternates: %w", err)
	}
	// Without --local, repack deliberately copies reachable borrowed objects
	// into the shadow object database.
	if _, err := s.git(ctx, gitDir, "", "repack", "-q", "-a", "-d"); err != nil {
		return err
	}
	if err := os.Rename(alternatesPath, pendingPath); err != nil {
		return fmt.Errorf("checkpoint: detach alternates: %w", err)
	}
	if err := s.verifyLocalObjects(ctx, gitDir); err != nil {
		if restoreErr := os.Rename(pendingPath, alternatesPath); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("checkpoint: restore alternates: %w", restoreErr))
		}
		return err
	}
	if err := os.Remove(pendingPath); err != nil {
		return fmt.Errorf("checkpoint: remove detached alternates: %w", err)
	}
	return nil
}

func (s *Store) verifyLocalObjects(ctx context.Context, gitDir string) error {
	if _, err := s.git(ctx, gitDir, "", "fsck", "--connectivity-only", "--no-dangling"); err != nil {
		return fmt.Errorf("checkpoint: verify local object graph: %w", err)
	}
	return nil
}

// tagFor maps a run id to its snapshot ref name, sanitizing any character git
// disallows in a ref so an unusual run id can't break tagging.
func tagFor(runID string) string {
	var b strings.Builder
	b.WriteString("chk/")
	for _, r := range runID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
