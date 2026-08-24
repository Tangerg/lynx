package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
)

// File browsing returns workspace entries for tree and completion consumers.
// Listing is gitignore-aware: in a git repo the
// candidate set comes from `git ls-files` (tracked + untracked-not-ignored, the
// repo's own .gitignore as authority); outside a repo it's a filesystem walk
// that skips a backstop set of heavy build/vcs dirs. This package owns Git
// interaction so callers depend only on listing behavior.

// EntryKind is a listed entry's type: file, directory, or symbolic link.
type EntryKind string

const (
	EntryFile    EntryKind = "file"
	EntryDir     EntryKind = "dir"
	EntrySymlink EntryKind = "symlink"
)

// FileEntry is one inspected entry, path relative to the workspace root
// (slash-separated). It owns the file facts needed by every listing consumer;
// callers don't need a second, potentially inconsistent stat pass.
type FileEntry struct {
	Path       string
	Name       string
	Kind       EntryKind
	SizeBytes  int64
	ModifiedAt time.Time
}

// OrderKey is the stable ordering and pagination identity for a listing.
// Directories sort before non-directories; paths break ties deterministically.
func (e FileEntry) OrderKey() string {
	class := "1"
	if e.Kind == EntryDir {
		class = "0"
	}
	return class + ":" + e.Path
}

// ListFilesOptions controls one listing. Path is a root-relative subdirectory
// already confined by the caller; empty selects the root.
type ListFilesOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

var (
	// ErrListingTooLarge asks the caller to narrow Path or Glob instead of
	// returning an incomplete result that looks authoritative.
	ErrListingTooLarge = errors.New("workspace: file listing too large")
	// ErrInvalidGlob distinguishes malformed match syntax from a valid pattern
	// that simply has no matching files.
	ErrInvalidGlob = errors.New("workspace: invalid file glob")
)

// maxListEntries is a safety boundary, not a silent result cap. Crossing it
// returns ErrListingTooLarge so callers can report precise invalid input.
const maxListEntries = 20000

// backstopExclude are directories never worth listing. `.git` is always
// skipped (even with includeIgnored — its internals are never useful); the
// rest are skipped only when not includeIgnored, as a coarse stand-in for
// .gitignore outside a git repo.
var backstopExclude = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "dist": true,
	"build": true, "target": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, ".idea": true, ".vscode": true,
	".cache": true, "coverage": true, ".turbo": true, ".svn": true, ".hg": true,
}

// ListFiles lists entries under opts.Path within root. With Recursive (or a
// Glob) it returns a flat list of files for the subtree; otherwise the
// immediate children (files + dirs) of opts.Path, for a lazy file tree.
// The complete, deterministically ordered result is returned for use-case
// pagination. Oversized trees fail explicitly with ErrListingTooLarge.
func ListFiles(ctx context.Context, root string, opts ListFilesOptions) ([]FileEntry, error) {
	sub := path.Clean(filepath.ToSlash(opts.Path))
	if sub == "." || sub == "/" {
		sub = ""
	}
	if opts.Glob != "" {
		if _, err := matchGlob(opts.Glob, ""); err != nil {
			return nil, fmt.Errorf("%w %q: %v", ErrInvalidGlob, opts.Glob, err)
		}
	}
	repository := false
	var files []string
	if !opts.IncludeIgnored {
		var err error
		files, err = git.ListFiles(ctx, root, sub, maxListEntries)
		switch {
		case err == nil:
			repository = true
		case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
			// Git-aware listing is unavailable for this workspace. The
			// filesystem fallback below remains authoritative in this case.
		case errors.Is(err, git.ErrResultTooLarge):
			return nil, fmt.Errorf("%w: more than %d files", ErrListingTooLarge, maxListEntries)
		default:
			return nil, err
		}
	}
	// A non-recursive filesystem listing is genuinely one level. Walking the
	// entire subtree first defeats lazy tree loading: a home-directory workspace
	// can hit an unreadable descendant or the global safety limit before its
	// immediate children are returned. Git-backed listings still derive their
	// children from `git ls-files` so ignored directories stay hidden.
	if !opts.Recursive && opts.Glob == "" && !repository {
		return levelFilesystemEntries(ctx, root, sub, opts.IncludeIgnored)
	}

	if !repository {
		var err error
		files, err = walkFiles(ctx, root, sub, opts.IncludeIgnored)
		if err != nil {
			return nil, err
		}
	}
	if len(files) > maxListEntries {
		return nil, fmt.Errorf("%w: more than %d files", ErrListingTooLarge, maxListEntries)
	}

	if opts.Recursive || opts.Glob != "" {
		return recursiveFiles(root, files, opts.Glob)
	}
	return levelEntries(root, files, sub)
}

func levelFilesystemEntries(ctx context.Context, root, sub string, includeIgnored bool) ([]FileEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := root
	if sub != "" {
		directory = filepath.Join(root, filepath.FromSlash(sub))
	}
	children, err := readDirectoryEntries(directory, maxListEntries)
	if err != nil {
		return nil, err
	}
	entries := make([]FileEntry, 0, len(children))
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if child.Name() == ".git" || (child.IsDir() && !includeIgnored && backstopExclude[child.Name()]) {
			continue
		}
		rel := path.Join(sub, child.Name())
		entry, exists, err := inspectEntry(root, rel)
		if err != nil {
			return nil, err
		}
		if exists {
			entries = append(entries, entry)
		}
	}
	sortFileEntries(entries)
	return entries, nil
}

func readDirectoryEntries(directory string, limit int) ([]fs.DirEntry, error) {
	dir, err := os.Open(directory)
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	defer dir.Close()

	// Read one sentinel entry beyond the contract limit. os.ReadDir(directory)
	// would materialize an unbounded directory before the safety policy had a
	// chance to reject it.
	children, err := dir.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("list %q: %w", directory, err)
	}
	if len(children) > limit {
		return nil, fmt.Errorf("%w: more than %d entries in %q", ErrListingTooLarge, limit, directory)
	}
	return children, nil
}

// walkFiles is the non-repo fallback: a filesystem walk under root/sub that
// skips backstop directories and fails explicitly at the safety boundary.
func walkFiles(ctx context.Context, root, sub string, includeIgnored bool) ([]string, error) {
	start := root
	if sub != "" {
		start = filepath.Join(root, filepath.FromSlash(sub))
	}
	var files []string
	walkErr := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return fmt.Errorf("visit %q: %w", p, err)
		}
		if p != start && d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != start && !includeIgnored && backstopExclude[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) > maxListEntries {
			return ErrListingTooLarge
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %q: %w", start, walkErr)
	}
	return files, nil
}

// recursiveFiles turns flat candidate paths into inspected file entries.
func recursiveFiles(root string, files []string, glob string) ([]FileEntry, error) {
	out := make([]FileEntry, 0, len(files))
	for _, f := range files {
		if glob != "" {
			matched, err := matchGlob(glob, f)
			if err != nil {
				return nil, fmt.Errorf("%w %q: %v", ErrInvalidGlob, glob, err)
			}
			if !matched {
				continue
			}
		}
		entry, exists, err := inspectEntry(root, f)
		if err != nil {
			return nil, err
		}
		if exists {
			out = append(out, entry)
		}
	}
	sortFileEntries(out)
	return out, nil
}

// levelEntries derives the immediate children of sub from the flat candidate
// paths: direct files become file entries, and any deeper path contributes its
// first path segment as a dir entry (deduped). Dirs sort before files.
func levelEntries(root string, files []string, sub string) ([]FileEntry, error) {
	prefix := ""
	if sub != "" {
		prefix = sub + "/"
	}
	seenDir := map[string]bool{}
	var children []string
	for _, f := range files {
		rel := f
		if prefix != "" {
			tail, ok := strings.CutPrefix(f, prefix)
			if !ok {
				continue
			}
			rel = tail
		}
		if name, _, nested := strings.Cut(rel, "/"); nested {
			if !seenDir[name] {
				seenDir[name] = true
				children = append(children, path.Join(sub, name))
			}
			continue
		}
		children = append(children, f)
	}
	entries := make([]FileEntry, 0, len(children))
	for _, child := range children {
		entry, exists, err := inspectEntry(root, child)
		if err != nil {
			return nil, err
		}
		if exists {
			entries = append(entries, entry)
		}
	}
	sortFileEntries(entries)
	return entries, nil
}

func inspectEntry(root, rel string) (FileEntry, bool, error) {
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		// git ls-files includes tracked deletions. They are not current workspace
		// entries, so omit them from the filesystem view.
		return FileEntry{}, false, nil
	}
	if err != nil {
		return FileEntry{}, false, fmt.Errorf("inspect %q: %w", rel, err)
	}

	kind := EntryFile
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = EntrySymlink
	case info.IsDir():
		kind = EntryDir
	}
	return FileEntry{
		Path:       rel,
		Name:       path.Base(rel),
		Kind:       kind,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	}, true, nil
}

func sortFileEntries(entries []FileEntry) {
	slices.SortFunc(entries, func(a, b FileEntry) int {
		return strings.Compare(a.OrderKey(), b.OrderKey())
	})
}

// matchGlob matches a doublestar-ish pattern against a slash path. Covers the
// shapes that actually occur ("**/*.go" → suffix on the basename, "*.ts" →
// basename, "src/*.go" → full path); not a complete doublestar engine.
func matchGlob(pattern, relPath string) (bool, error) {
	if rest, ok := strings.CutPrefix(pattern, "**/"); ok {
		return path.Match(rest, path.Base(relPath))
	}
	if strings.Contains(pattern, "/") {
		return path.Match(pattern, relPath)
	}
	return path.Match(pattern, path.Base(relPath))
}
