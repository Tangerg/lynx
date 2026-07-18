package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
)

// File browsing (workspace.listFiles, API.md §7.5) — the file-tree browser +
// @file autocomplete source. Listing is gitignore-aware: in a git repo the
// candidate set comes from `git ls-files` (tracked + untracked-not-ignored, the
// repo's own .gitignore as authority); outside a repo it's a filesystem walk
// that skips a backstop set of heavy build/vcs dirs. This lives in the adapter
// layer so delivery never imports infra/git directly.

// EntryKind is a listed entry's type — file / dir / symlink (wire §7.5).
type EntryKind string

const (
	EntryFile    EntryKind = "file"
	EntryDir     EntryKind = "dir"
	EntrySymlink EntryKind = "symlink"
)

// FileEntry is one inspected entry, path relative to the workspace root
// (slash-separated). It owns the file facts needed by every delivery adapter;
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

// ListFilesOptions mirrors the workspace.listFiles params (§7.5). Path is a
// root-relative sub-directory (already jailed by the caller); empty = root.
type ListFilesOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

// ErrListingTooLarge asks the caller to narrow Path or Glob instead of
// returning an incomplete result that looks authoritative.
var ErrListingTooLarge = errors.New("workspace: file listing too large")

// maxListEntries is a safety boundary, not a silent result cap. Crossing it
// returns ErrListingTooLarge so delivery can surface a clear invalid_params.
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
// The complete, deterministically ordered result is returned for application-
// level pagination. Oversized trees fail explicitly with ErrListingTooLarge.
func ListFiles(ctx context.Context, root string, opts ListFilesOptions) ([]FileEntry, error) {
	sub := path.Clean(filepath.ToSlash(opts.Path))
	if sub == "." || sub == "/" {
		sub = ""
	}

	files, err := candidateFiles(ctx, root, sub, opts.IncludeIgnored)
	if err != nil {
		return nil, err
	}
	if len(files) > maxListEntries {
		return nil, fmt.Errorf("%w: more than %d files", ErrListingTooLarge, maxListEntries)
	}

	if opts.Recursive || opts.Glob != "" {
		return recursiveFiles(root, files, opts.Glob)
	}
	return levelEntries(root, files, sub)
}

// candidateFiles returns every non-ignored file under sub (relative to root),
// gitignore-aware via `git ls-files` in a repo, else a bounded filesystem walk.
func candidateFiles(ctx context.Context, root, sub string, includeIgnored bool) ([]string, error) {
	if !includeIgnored && git.IsRepo(ctx, root) {
		return git.ListFiles(ctx, root, sub)
	}
	return walkFiles(ctx, root, sub, includeIgnored)
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
		if d.IsDir() {
			if p != start && (d.Name() == ".git" || (!includeIgnored && backstopExclude[d.Name()])) {
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
		if glob != "" && !matchGlob(glob, f) {
			continue
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
func matchGlob(pattern, relPath string) bool {
	if rest, ok := strings.CutPrefix(pattern, "**/"); ok {
		matched, _ := path.Match(rest, path.Base(relPath))
		return matched
	}
	if strings.Contains(pattern, "/") {
		matched, _ := path.Match(pattern, relPath)
		return matched
	}
	matched, _ := path.Match(pattern, path.Base(relPath))
	return matched
}
