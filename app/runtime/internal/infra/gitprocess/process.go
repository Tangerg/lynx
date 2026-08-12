// Package gitprocess owns the operating-system process boundary for local Git
// commands. Runtime commands always name their repository or work tree
// explicitly, so inheriting a parent process's GIT_* control plane would make
// their target and semantics ambiguous.
package gitprocess

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Environment returns the host environment without inherited Git controls,
// followed by the command-owned overrides. Clearing the namespace as a whole
// covers repository routing, alternate object/index files, replace refs,
// injected config, pathspec modes, and future Git controls without maintaining
// a version-sensitive denylist. Ordinary Git config discovery through HOME and
// the explicitly selected repository remains available unless the caller
// overrides it.
func Environment(overrides ...string) []string {
	overridden := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		if key, _, ok := strings.Cut(entry, "="); ok {
			overridden[environmentKey(key)] = struct{}{}
		}
	}

	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || hasGitPrefix(key) {
			continue
		}
		if _, replaced := overridden[environmentKey(key)]; replaced {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, overrides...)
}

func hasGitPrefix(key string) bool {
	return len(key) >= len("GIT_") && strings.EqualFold(key[:len("GIT_")], "GIT_")
}

func environmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

// Command constructs a Git process that cannot inherit repository-local state
// or behavior from its parent.
func Command(args ...string) *exec.Cmd {
	command := exec.Command("git", args...)
	command.Env = Environment()
	return command
}

// CommandContext is Command with context-driven cancellation.
func CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = Environment()
	return command
}
