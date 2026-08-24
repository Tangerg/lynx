// Package promptsource is the filesystem adapter for the prompt-source domains:
// it discovers the AGENTS.md, skill, and recipe files a session exposes, walking
// the project tree and the well-known user-level directories. The precedence,
// render, and parse RULES are the domains' (agentdoc / skills / recipes); the
// file discovery and reads are here (§4.5).
package promptsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
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
// ctx cancels long walks; cwd / home are absolute composition inputs. Missing
// and empty files contribute nothing. An existing unreadable, invalid, or
// oversized document rejects the complete cascade so management and execution
// cannot observe different instruction sets. The agent-execution adapter
// renders the resulting values.
func DiscoverAgentDocs(ctx context.Context, cwd, home string) ([]workspaceapp.AgentDocFile, error) {
	if cwd == "" {
		return nil, errors.New("promptsource: cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		return nil, errors.New("promptsource: cwd must be absolute")
	}
	if home != "" && !filepath.IsAbs(home) {
		return nil, errors.New("promptsource: home must be absolute")
	}
	cwd = filepath.Clean(cwd)

	d := &agentDocScan{seen: make(map[string]struct{})}

	// 1) User-level: Lyra-specific first, then generic (first-match).
	if home != "" {
		if err := d.try(ctx, filepath.Join(home, ".lyra", "AGENTS.md"), workspaceapp.AgentDocScopeHome); err != nil {
			return nil, err
		}
		if err := d.tryFirst(ctx, workspaceapp.AgentDocScopeHome,
			filepath.Join(home, ".agents", "AGENTS.md"),
			filepath.Join(home, ".agents", "agents.md"),
		); err != nil {
			return nil, err
		}
	}

	// 2) Project tree: root → leaf so deeper files end the blob.
	root := findProjectRoot(cwd)
	for _, dir := range dirsRootToLeaf(cwd, root) {
		scope := workspaceapp.AgentDocScopeProjectRoot
		if dir == cwd {
			scope = workspaceapp.AgentDocScopeCWD
		}
		if err := d.try(ctx, filepath.Join(dir, ".lyra", "AGENTS.md"), scope); err != nil {
			return nil, err
		}
		if err := d.tryFirst(ctx, scope,
			filepath.Join(dir, "AGENTS.md"),
			filepath.Join(dir, "agents.md"),
		); err != nil {
			return nil, err
		}
	}

	return d.files, nil
}

// AgentDocs adapts prompt-source discovery to the workspace application port.
type AgentDocs struct{}

func (AgentDocs) Find(ctx context.Context, cwd, home string) ([]workspaceapp.AgentDocFile, error) {
	return DiscoverAgentDocs(ctx, cwd, home)
}

// agentDocScan carries de-duplication and complete-cascade admission across the
// walk. Missing/empty candidates remain absent; an existing invalid candidate
// returns its error instead of silently changing the instruction set.
type agentDocScan struct {
	seen     map[string]struct{}
	files    []workspaceapp.AgentDocFile
	rawBytes int
}

func (d *agentDocScan) try(ctx context.Context, path string, scope workspaceapp.AgentDocScope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	clean := filepath.Clean(path)
	if _, err := os.Lstat(clean); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("promptsource: inspect agent document %q: %w", path, err)
	}
	abs, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return fmt.Errorf("promptsource: resolve agent document %q: %w", path, err)
	}
	if _, dup := d.seen[abs]; dup {
		return nil
	}
	content, size, ok, err := readIfNonEmpty(ctx, abs)
	if err != nil {
		return fmt.Errorf("promptsource: read agent document: %w", err)
	}
	if !ok {
		return nil
	}
	if len(d.files) >= workspaceapp.MaxAgentDocumentsPerCascade {
		return fmt.Errorf(
			"%w: agent document cascade has more than %d documents",
			workspaceapp.ErrPromptSourceTooLarge,
			workspaceapp.MaxAgentDocumentsPerCascade,
		)
	}
	if size > workspaceapp.MaxAgentDocumentCascadeBytes-d.rawBytes {
		return fmt.Errorf(
			"%w: agent document cascade exceeds %d bytes",
			workspaceapp.ErrPromptSourceTooLarge,
			workspaceapp.MaxAgentDocumentCascadeBytes,
		)
	}
	d.seen[abs] = struct{}{}
	d.rawBytes += size
	d.files = append(d.files, workspaceapp.AgentDocFile{Path: abs, Content: content, Scope: scope})
	return nil
}

func (d *agentDocScan) tryFirst(ctx context.Context, scope workspaceapp.AgentDocScope, candidates ...string) error {
	for _, c := range candidates {
		before := len(d.files)
		if err := d.try(ctx, c, scope); err != nil {
			return err
		}
		if len(d.files) > before {
			return nil
		}
	}
	return nil
}

// readIfNonEmpty returns trimmed, admitted content and its complete encoded
// size. Empty files remain absent; an existing invalid file remains observable.
func readIfNonEmpty(ctx context.Context, path string) (string, int, bool, error) {
	data, err := readAuthoredPromptFile(ctx, path)
	if err != nil {
		return "", 0, false, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", len(data), false, nil
	}
	return content, len(data), true, nil
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
