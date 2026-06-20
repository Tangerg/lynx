package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// hooksFile is the cascade filename. Global lives at ~/.lyra/hooks.json; a
// project's lives at <dir>/.lyra/hooks.json for any dir from the project root
// down to the cwd (nested, like the AGENTS.md cascade).
const hooksRelPath = ".lyra/hooks.json"

// Load discovers and parses the hooks.json cascade for a working directory and
// returns every configured hook, each stamped with its [Scope] (global vs
// project) and Source path. Order is global first, then project root→cwd, so
// the [Runner]'s deterministic combine (first-block / first-rewrite wins) favors
// the user's own global hooks over a repo's.
//
//  1. ~/.lyra/hooks.json                          → ScopeGlobal
//  2. {dir}/.lyra/hooks.json for dir in root…cwd  → ScopeProject
//
// Trust is NOT enforced here — Load reports provenance; the caller drops
// ScopeProject hooks for an untrusted project (a cloned repo's hooks must not
// auto-run). Missing/empty files are skipped silently; a malformed one is
// skipped and reported via onParseError (best-effort: a broken hooks.json must
// not fail a turn). home / cwd are absolute (caller resolves).
func Load(ctx context.Context, cwd, home string, onParseError func(path string, err error)) ([]Hook, error) {
	if cwd == "" {
		return nil, errors.New("hooks: cwd is required")
	}
	cwd = filepath.Clean(cwd)

	var out []Hook
	seen := make(map[string]struct{})
	add := func(path string, scope Scope) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		if _, dup := seen[abs]; dup {
			return nil
		}
		seen[abs] = struct{}{}
		cfg, ok, perr := readConfig(abs)
		if perr != nil {
			if onParseError != nil {
				onParseError(abs, perr)
			}
			return nil
		}
		if !ok {
			return nil
		}
		for _, h := range cfg.Hooks {
			h.Scope = scope
			h.Source = abs
			out = append(out, h)
		}
		return nil
	}

	if home != "" {
		if err := add(filepath.Join(home, hooksRelPath), ScopeGlobal); err != nil {
			return nil, err
		}
	}
	for _, dir := range dirsRootToLeaf(cwd, ProjectRoot(cwd)) {
		if err := add(filepath.Join(dir, hooksRelPath), ScopeProject); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readConfig reads + parses one hooks.json. ok=false for a missing / empty file
// (not an error); a present-but-malformed file returns the parse error.
func readConfig(path string) (Config, bool, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Config{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false, nil
	}
	if len(data) == 0 {
		return Config{}, false, nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

// ProjectRoot returns cwd's project root — the nearest ancestor with a `.git`
// entry (dir or file, for submodules/worktrees), or cwd when none is found. It's
// the trust key: the caller asks "is THIS project trusted?" before running its
// project-scope hooks. Mirrors the AGENTS.md discovery root.
func ProjectRoot(cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd)
		}
		current = parent
	}
}

// dirsRootToLeaf returns [root … cwd] inclusive (one element when equal).
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
