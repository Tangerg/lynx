// Package promptsource is the filesystem adapter for the prompt-source domains:
// it discovers the AGENTS.md, skill, and recipe files a session exposes, walking
// the project tree and the well-known user-level directories. The precedence,
// render, and parse RULES are the domains' (agentdoc / skills / recipes); the
// file discovery and reads are here (§4.5).
package promptsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
)

// DiscoverAgentDocs walks the project tree + user-level locations and returns
// the AGENTS.md files in render order:
//
//  1. ~/.lyra/AGENTS.md           (Lyra-specific user scope)
//  2. ~/.agents/AGENTS.md         (cross-tool generic; first match
//     of AGENTS.md / agents.md)
//  3. for each dir from project-root → cwd inclusive:
//     - {dir}/.lyra/AGENTS.md     (Lyra subdir convention)
//     - {dir}/AGENTS.md           (first match of AGENTS.md / agents.md)
//
// Project root = the nearest ancestor containing a `.git` entry; if none is
// found, root = cwd (single-level scan). Symlinked / case-folded duplicate paths
// are deduped by absolute path.
//
// ctx cancels long walks; cwd / home are absolute (callers resolve via
// os.UserHomeDir / os.Getwd before calling). Missing files are never an error —
// discovery is best-effort. The rendered blob is produced by [agentdoc.Render].
func DiscoverAgentDocs(ctx context.Context, cwd, home string) ([]agentdoc.File, error) {
	if cwd == "" {
		return nil, errors.New("promptsource: cwd is required")
	}
	cwd = filepath.Clean(cwd)

	d := &agentDocScan{seen: make(map[string]struct{})}

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

// agentDocScan carries the de-dup state across the walk. Methods return the ctx
// error and nothing else — file-stat failures and empty files are silently
// skipped per the best-effort policy.
type agentDocScan struct {
	seen  map[string]struct{}
	files []agentdoc.File
}

func (d *agentDocScan) try(ctx context.Context, path string) error {
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
	d.files = append(d.files, agentdoc.File{Path: abs, Content: content})
	return nil
}

func (d *agentDocScan) tryFirst(ctx context.Context, candidates ...string) error {
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

// readIfNonEmpty reads path and returns its trimmed content + true when the file
// exists and has content. Errors (incl. ENOENT) and empty files return ok=false
// silently.
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

// findProjectRoot walks up from cwd looking for a `.git` entry (dir OR file —
// submodules use `.git` files pointing to the real gitdir). Returns cwd unchanged
// if no .git is found anywhere on the way up (single-dir scan).
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

// dirsRootToLeaf returns the chain [root, ..., cwd] (inclusive at both ends).
// When root == cwd the slice has one element.
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
