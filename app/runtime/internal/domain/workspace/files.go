package workspace

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
)

// File browsing (workspace.listFiles, API.md §7.5) — the file-tree browser +
// @file autocomplete source. Listing is gitignore-aware: in a git repo the
// candidate set comes from `git ls-files` (tracked + untracked-not-ignored, the
// repo's own .gitignore as authority); outside a repo it's a filesystem walk
// that skips a backstop set of heavy build/vcs dirs. This lives in the domain
// (not delivery) so delivery never imports infra/git directly.

// EntryKind is a listed entry's type — file / dir / symlink (wire §7.5).
type EntryKind string

const (
	EntryFile    EntryKind = "file"
	EntryDir     EntryKind = "dir"
	EntrySymlink EntryKind = "symlink"
)

// FileEntry is one listed entry, path relative to the workspace root
// (slash-separated). Size/mtime are intentionally not populated — the
// consumers (file tree + @file) don't need them, and statting every entry of a
// recursive list would dominate the call; the wire fields stay optional.
type FileEntry struct {
	Path string
	Name string
	Kind EntryKind
}

// ListFilesOptions mirrors the workspace.listFiles params (§7.5). Path is a
// root-relative sub-directory (already jailed by the caller); empty = root.
type ListFilesOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
	Limit          int
}

const (
	defaultListLimit = 1000
	// maxWalk bounds the non-repo filesystem fallback so a session opened on a
	// huge non-git directory can't turn one listing into a multi-second walk.
	maxWalk = 20000
)

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
// Returns the (possibly capped) entries and whether the result was truncated.
func ListFiles(ctx context.Context, root string, opts ListFilesOptions) ([]FileEntry, bool, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	sub := path.Clean(filepath.ToSlash(opts.Path))
	if sub == "." || sub == "/" {
		sub = ""
	}

	files, walkTrunc, err := candidateFiles(ctx, root, sub, opts.IncludeIgnored)
	if err != nil {
		return nil, false, err
	}

	if opts.Recursive || opts.Glob != "" {
		entries, truncated := recursiveFiles(files, opts.Glob, limit, walkTrunc)
		return entries, truncated, nil
	}
	entries, truncated := levelEntries(files, sub, limit, walkTrunc)
	return entries, truncated, nil
}

// candidateFiles returns every non-ignored file under sub (relative to root),
// gitignore-aware via `git ls-files` in a repo, else a bounded filesystem walk.
func candidateFiles(ctx context.Context, root, sub string, includeIgnored bool) ([]string, bool, error) {
	if !includeIgnored && git.IsRepo(ctx, root) {
		files, err := git.ListFiles(ctx, root, sub)
		return files, false, err
	}
	return walkFiles(root, sub, includeIgnored)
}

// walkFiles is the non-repo fallback: a filesystem walk under root/sub that
// skips backstop directories and stops at maxWalk (reporting truncation).
func walkFiles(root, sub string, includeIgnored bool) ([]string, bool, error) {
	start := root
	if sub != "" {
		start = filepath.Join(root, filepath.FromSlash(sub))
	}
	var files []string
	truncated := false
	walkErr := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the whole walk
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
		if len(files) >= maxWalk {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	return files, truncated, walkErr
}

// recursiveFiles turns the flat candidate paths into file entries, applying the
// glob filter and the limit.
func recursiveFiles(files []string, glob string, limit int, walkTrunc bool) ([]FileEntry, bool) {
	out := make([]FileEntry, 0, min(len(files), limit))
	truncated := walkTrunc
	for _, f := range files {
		if glob != "" && !matchGlob(glob, f) {
			continue
		}
		if len(out) >= limit {
			truncated = true
			break
		}
		out = append(out, FileEntry{Path: f, Name: path.Base(f), Kind: EntryFile})
	}
	return out, truncated
}

// levelEntries derives the immediate children of sub from the flat candidate
// paths: direct files become file entries, and any deeper path contributes its
// first path segment as a dir entry (deduped). Dirs sort before files.
func levelEntries(files []string, sub string, limit int, walkTrunc bool) ([]FileEntry, bool) {
	prefix := ""
	if sub != "" {
		prefix = sub + "/"
	}
	seenDir := map[string]bool{}
	var dirs, plain []FileEntry
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
				dirs = append(dirs, FileEntry{Path: path.Join(sub, name), Name: name, Kind: EntryDir})
			}
			continue
		}
		plain = append(plain, FileEntry{Path: f, Name: rel, Kind: EntryFile})
	}
	byName := func(a, b FileEntry) int { return strings.Compare(a.Name, b.Name) }
	slices.SortFunc(dirs, byName)
	slices.SortFunc(plain, byName)
	entries := append(dirs, plain...)
	truncated := walkTrunc
	if len(entries) > limit {
		entries = entries[:limit]
		truncated = true
	}
	return entries, truncated
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
