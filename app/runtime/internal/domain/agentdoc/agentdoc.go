// Package agentdoc discovers AGENTS.md files in the project tree
// and renders them into a single, budgeted blob the engine injects
// into the system prompt.
//
// The AGENTS.md convention (https://agents.md) is a cross-tool
// markdown file checked into a repo that briefs any AI coding agent
// about the project: stack, conventions, gotchas, commands. Lyra
// walks from the working directory up to the project root, plus a
// couple of well-known user-level paths, so notes nested in a
// subdir take precedence over notes at the repo root.
//
// LYRA.md is intentionally NOT in scope here — that's managed by
// internal/domain/knowledge (writable via the runtime protocol).
// AGENTS.md is read-only at runtime; the engine never writes to it.
package agentdoc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DefaultMaxBytes caps the rendered blob — 32 KiB, generous enough
// for several layers of nested AGENTS.md, tight enough to not blow the
// system prompt budget.
const DefaultMaxBytes = 32 * 1024

// File is one discovered AGENTS.md (or agents.md) with its absolute
// path. Path is the source-of-truth used in render annotations so
// the model can attribute claims back to the right file.
type File struct {
	Path    string
	Content string
}

// Discover walks the project tree + user-level locations and returns
// the AGENTS.md files in render order:
//
//  1. ~/.lyra/AGENTS.md           (Lyra-specific user scope)
//  2. ~/.agents/AGENTS.md         (cross-tool generic; first match
//     of AGENTS.md / agents.md)
//  3. for each dir from project-root → cwd inclusive:
//     - {dir}/.lyra/AGENTS.md     (Lyra subdir convention)
//     - {dir}/AGENTS.md           (first match of AGENTS.md / agents.md)
//
// Project root = the nearest ancestor containing a `.git` entry; if
// none is found, root = cwd (single-level scan). Symlinked /
// case-folded duplicate paths are deduped by absolute path.
//
// ctx cancels long walks; cwd / home are absolute (callers resolve
// via os.UserHomeDir / os.Getwd before calling). Missing files are
// never an error — discovery is best-effort.
func Discover(ctx context.Context, cwd, home string) ([]File, error) {
	if cwd == "" {
		return nil, errors.New("agentdoc: cwd is required")
	}
	cwd = filepath.Clean(cwd)

	d := &discoverer{seen: make(map[string]struct{})}

	// 1) User-level: Lyra-specific first, then generic (first-match).
	if home != "" {
		if err := d.try(ctx, filepath.Join(home, ".lyra", "AGENTS.md")); err != nil {
			return nil, err
		}
		if err := d.tryFirst(ctx,
			filepath.Join(home, ".agents", "AGENTS.md"),
			filepath.Join(home, ".agents", "agents.md"),
		); err != nil {
			return nil, err
		}
	}

	// 2) Project tree: root → leaf so deeper files end the blob.
	root := findProjectRoot(cwd)
	for _, dir := range dirsRootToLeaf(cwd, root) {
		if err := d.try(ctx, filepath.Join(dir, ".lyra", "AGENTS.md")); err != nil {
			return nil, err
		}
		if err := d.tryFirst(ctx,
			filepath.Join(dir, "AGENTS.md"),
			filepath.Join(dir, "agents.md"),
		); err != nil {
			return nil, err
		}
	}

	return d.files, nil
}

// discoverer carries the de-dup state across the call. Methods
// return the ctx error and nothing else — file-stat failures and
// empty files are silently skipped per the best-effort policy.
type discoverer struct {
	seen  map[string]struct{}
	files []File
}

// try reads path and appends to files when the file exists, is
// non-empty, and hasn't already been recorded under its absolute
// path. Returns ctx.Err() so cancellation propagates.
func (d *discoverer) try(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	if _, dup := d.seen[abs]; dup {
		return nil
	}
	content, ok := readIfNonEmpty(abs)
	if !ok {
		return nil
	}
	d.seen[abs] = struct{}{}
	d.files = append(d.files, File{Path: abs, Content: content})
	return nil
}

// tryFirst walks candidates in order, stopping at the first one
// that successfully records (file existed + had content). Used for
// the AGENTS.md vs agents.md pair on case-sensitive filesystems.
func (d *discoverer) tryFirst(ctx context.Context, candidates ...string) error {
	for _, c := range candidates {
		before := len(d.files)
		if err := d.try(ctx, c); err != nil {
			return err
		}
		if len(d.files) > before {
			return nil
		}
	}
	return nil
}

// readIfNonEmpty reads path and returns its trimmed content + true
// when the file exists and has content. Errors (incl. ENOENT) and
// empty files return ok=false silently.
func readIfNonEmpty(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}
	return content, true
}

// findProjectRoot walks up from cwd looking for a `.git` entry
// (dir OR file — submodules use `.git` files pointing to the real
// gitdir). Returns cwd unchanged if no .git is found anywhere on
// the way up (single-dir scan).
func findProjectRoot(cwd string) string {
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cwd
		}
		current = parent
	}
}

// dirsRootToLeaf returns the chain [root, ..., cwd] (inclusive at
// both ends). When root == cwd the slice has one element.
func dirsRootToLeaf(cwd, root string) []string {
	if cwd == root {
		return []string{cwd}
	}
	var chain []string
	current := cwd
	for current != root {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	chain = append(chain, root)
	slices.Reverse(chain)
	return chain
}

// Render concatenates files into a single blob with
// `<!-- From: /path -->` provenance headers. When the byte budget
// is exceeded, files at the front of the list (root-most) are
// dropped first — they're the least specific so most expendable.
// Returns "" when no files fit or the input is empty.
func Render(files []File, maxBytes int) string {
	if len(files) == 0 || maxBytes <= 0 {
		return ""
	}

	blocks := make([]string, len(files))
	sizes := make([]int, len(files))
	total := 0
	for i, f := range files {
		blocks[i] = annotation(f.Path) + f.Content + "\n"
		sizes[i] = len(blocks[i])
		total += sizes[i]
	}
	// Inter-block separator (one blank line between blocks).
	if len(files) > 1 {
		total += len(files) - 1
	}

	start := 0
	for start < len(files) && total > maxBytes {
		total -= sizes[start]
		if start > 0 {
			total-- // remove the separator that was between start-1 and start
		}
		start++
	}
	if start >= len(files) {
		return ""
	}

	var b strings.Builder
	b.Grow(total)
	for i := start; i < len(files); i++ {
		if i > start {
			b.WriteString("\n")
		}
		b.WriteString(blocks[i])
	}
	return b.String()
}

// annotation is the per-file header. Keep it on one line + trailing
// LF so the model reads the path inline without extra blank.
func annotation(path string) string {
	return "<!-- From: " + path + " -->\n"
}
