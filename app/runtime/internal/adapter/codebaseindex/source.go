package codebaseindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
)

// Build caps — bound an index pass so a huge repo can't run away (cost + memory).
const (
	maxFileBytes = 256 * 1024 // skip files larger than this (generated / minified / data)
	maxFiles     = 4000       // cap on indexed files (Truncated when hit)
	chunkLines   = 50         // lines per chunk window
	chunkOverlap = 10         // lines shared between adjacent windows (keeps context across cuts)
)

// Source discovers and chunks indexable code files from a project directory.
type Source struct{}

var _ domain.Source = Source{}

// codeExtensions is the allowlist of source extensions worth indexing — keeps
// the index code-centric (not data / lock / binary files) and bounds cost.
var codeExtensions = map[string]struct{}{
	".go": {}, ".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".py": {}, ".rs": {},
	".java": {}, ".kt": {}, ".c": {}, ".h": {}, ".cc": {}, ".cpp": {}, ".hpp": {},
	".cs": {}, ".rb": {}, ".php": {}, ".swift": {}, ".scala": {}, ".sh": {},
	".sql": {}, ".proto": {}, ".md": {}, ".css": {}, ".scss": {}, ".vue": {},
	".lua": {}, ".dart": {}, ".ex": {}, ".exs": {}, ".clj": {}, ".hs": {},
}

// skipDirs are noise directories the filesystem-walk fallback never descends
// into (git ls-files already excludes most via .gitignore).
var skipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, "vendor": {}, "dist": {}, "build": {},
	"target": {}, ".next": {}, ".venv": {}, "__pycache__": {}, ".idea": {},
	".vscode": {}, "out": {}, "bin": {}, "obj": {}, ".cache": {},
}

// Files lists the project's indexable source files (relative, slash
// paths). Gitignore-aware via `git ls-files` in a repo; a bounded filesystem
// walk (skipDirs) outside one. truncated reports the maxFiles cap was hit.
func (Source) Files(ctx context.Context, cwd string) (files []string, truncated bool, err error) {
	listed, gerr := git.ListFiles(ctx, cwd, ".")
	if gerr != nil {
		// Not a repo / git unavailable → filesystem walk fallback.
		listed, err = walkFiles(cwd)
		if err != nil {
			return nil, false, err
		}
	}
	out := make([]string, 0, len(listed))
	for _, rel := range listed {
		if !indexable(rel) {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) >= maxFiles {
			truncated = true
			break
		}
	}
	return out, truncated, nil
}

// indexable reports whether a path's extension is in the source allowlist.
func indexable(rel string) bool {
	_, ok := codeExtensions[strings.ToLower(filepath.Ext(rel))]
	return ok
}

func walkFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the walk
		}
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip {
				return fs.SkipDir
			}
			return nil
		}
		if rel, rerr := filepath.Rel(root, p); rerr == nil {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

// Chunks reads cwd/rel and splits it into overlapping line-window chunks
// (1-based inclusive line ranges) plus the file's content hash. ok=false when
// the file is unreadable, oversized, or binary — skipped without an error.
// Embeddings are left empty; the caller fills them after embedding.
func (Source) Chunks(cwd, rel string) (chunks []domain.Chunk, hash string, ok bool) {
	full := filepath.Join(cwd, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return nil, "", false
	}
	data, err := os.ReadFile(full)
	if err != nil || isBinary(data) {
		return nil, "", false
	}
	sum := sha256.Sum256(data)
	hash = hex.EncodeToString(sum[:])

	lines := strings.Split(string(data), "\n")
	for start := 0; start < len(lines); start += chunkLines - chunkOverlap {
		end := min(start+chunkLines, len(lines))
		if text := strings.TrimSpace(strings.Join(lines[start:end], "\n")); text != "" {
			chunks = append(chunks, domain.Chunk{Path: rel, StartLine: start + 1, EndLine: end, Text: text})
		}
		if end == len(lines) {
			break
		}
	}
	return chunks, hash, true
}

// isBinary reports whether data looks non-text (a NUL byte in the first 8KB).
func isBinary(data []byte) bool {
	for i := 0; i < min(len(data), 8192); i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
