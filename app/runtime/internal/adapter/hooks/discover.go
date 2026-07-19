package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// hooksRelPath is the cascade filename. Global lives at ~/.lyra/hooks.json; a
// project's lives at <dir>/.lyra/hooks.json for any dir from the project root
// down to the cwd.
const hooksRelPath = ".lyra/hooks.json"

// Load discovers and parses the hooks.json cascade for a working directory and
// returns every configured hook, each stamped with its scope and source path.
func Load(ctx context.Context, cwd, home string) ([]domainhooks.Hook, error) {
	return load(ctx, cwd, home, true)
}

// load can exclude project hooks at the trust boundary. An untrusted project's
// config is neither executed nor allowed to break otherwise-valid global hooks;
// management inspection calls Load and still validates the complete cascade.
func load(ctx context.Context, cwd, home string, includeProject bool) ([]domainhooks.Hook, error) {
	if cwd == "" {
		return nil, errors.New("hooks: cwd is required")
	}
	cwd = filepath.Clean(cwd)

	var out []domainhooks.Hook
	seen := make(map[string]struct{})
	add := func(path string, scope domainhooks.Scope) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("hooks: resolve config path %q: %w", path, err)
		}
		if _, dup := seen[abs]; dup {
			return nil
		}
		seen[abs] = struct{}{}
		cfg, ok, err := readConfig(abs)
		if err != nil {
			return fmt.Errorf("hooks: load config %q: %w", abs, err)
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
		if err := add(filepath.Join(home, hooksRelPath), domainhooks.ScopeGlobal); err != nil {
			return nil, err
		}
	}
	if includeProject {
		for _, dir := range dirsRootToLeaf(cwd, ProjectRoot(cwd)) {
			if err := add(filepath.Join(dir, hooksRelPath), domainhooks.ScopeProject); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

type config struct {
	Hooks []domainhooks.Hook `json:"hooks"`
}

func readConfig(path string) (config, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return config{}, false, nil
	}
	if err != nil {
		return config{}, false, err
	}
	if !info.Mode().IsRegular() {
		return config{}, false, errors.New("not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, false, err
	}
	if len(data) == 0 {
		return config{}, false, nil
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, false, err
	}
	for index, hook := range cfg.Hooks {
		if err := hook.Validate(); err != nil {
			return config{}, false, fmt.Errorf("hook %d: %w", index, err)
		}
	}
	return cfg, true, nil
}

// ProjectRoot returns cwd's project root, the nearest ancestor with a `.git`
// entry, or cwd when none is found. This is the project hook trust key.
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
