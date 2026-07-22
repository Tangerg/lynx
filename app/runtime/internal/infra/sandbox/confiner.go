// Package sandbox provides the runtime's OS command-isolation capabilities on a
// macOS Seatbelt backend (fail-closed elsewhere). It offers two models that
// share one confinement core (the SBPL profile + scrubbed environment):
//
//   - Confiner (wired): an in-place jail. A shell command runs against the real
//     working tree, confined to write only within its cwd, with the network
//     denied, $HOME hidden, and the environment scrubbed. It backs the live
//     shell tool (see internal/infra/exec).
//   - Workspace (wired): an isolated working copy with content-addressed tar
//     snapshots — a session marked Isolated runs its tools inside one instead of
//     the real tree (see internal/adapter/isolation). New/Path/Stop/Shutdown are
//     the copy lifecycle the isolation adapter drives; Resume rebuilds a copy
//     from a snapshot. macOS-only today (fail-closed elsewhere); its execpolicy /
//     Linux-Windows-backend extensions remain future work (the C7 backlog).
package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Command is a fully specified process to spawn under confinement: the program
// to run, its arguments, and the environment to run it with. The caller owns
// process lifecycle (start, wait, cancel, streaming) and sets the working
// directory to the confinement root. A nil Env means "inherit the parent's",
// which a Confiner never returns — it always scrubs.
type Command struct {
	Name string
	Args []string
	Env  []string
}

// Confiner applies the runtime's OS command-isolation policy in place: a command
// run through it can only write within a given root, cannot reach the network,
// cannot read the real $HOME, and runs with a scrubbed environment. It carries
// the reusable read-only policy — the writable root varies per command, so it is
// supplied to [Confiner.Confine], not held here.
//
// It is the in-place counterpart to [Workspace], which runs commands inside an
// isolated copy: a Confiner leaves the caller's working tree in place and only
// jails the process. The zero value is not usable; build one with [NewConfiner].
type Confiner struct {
	readOnlyPaths []string
	hiddenPaths   []string
}

// NewConfiner builds a command confiner that re-opens readOnlyPaths for reads
// below the hidden home (e.g. a language toolchain or dependency cache under
// $HOME a build needs). It fails closed with [ErrUnavailable] when the host has
// no supported isolation backend — only macOS Seatbelt today — so a caller that
// asked for confinement is refused at construction rather than silently running
// unconfined. Resolving a non-existent read-only path is likewise an error: an
// unreachable entry in a security policy is a configuration mistake.
func NewConfiner(readOnlyPaths []string) (*Confiner, error) {
	if err := checkBackend(); err != nil {
		return nil, err
	}
	readable, err := resolveReadOnlyPaths(readOnlyPaths)
	if err != nil {
		return nil, err
	}
	return &Confiner{readOnlyPaths: readable, hiddenPaths: hiddenHomePaths()}, nil
}

// Confine returns command as a [Command] jailed to root: root is its only
// writable subtree, networking is denied, the real $HOME is hidden, and the
// environment is scrubbed. root must resolve to an existing directory (it is the
// command's working directory and sole writable grant).
func (c *Confiner) Confine(root, command string) (Command, error) {
	if command == "" {
		return Command{}, errors.New("sandbox: command is required")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Command{}, fmt.Errorf("sandbox: resolve root: %w", err)
	}
	return seatbeltCommand(realRoot, command, c.readOnlyPaths, c.hiddenPaths), nil
}

// resolveReadOnlyPaths canonicalizes the caller-declared read-only roots
// (absolute + symlink-resolved) that a jail re-opens for reads below the hidden
// home. A non-resolvable entry is an error, not a silent drop.
func resolveReadOnlyPaths(in []string) ([]string, error) {
	paths := make([]string, 0, len(in))
	for _, value := range in {
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve read-only path %q: %w", value, err)
		}
		realPath, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve read-only path %q: %w", value, err)
		}
		paths = append(paths, realPath)
	}
	return paths, nil
}

// hiddenHomePaths returns the resolved real home directory to hide from reads,
// or nil when it can't be resolved (non-fatal: the environment is scrubbed and
// $HOME re-pointed at the workspace regardless).
func hiddenHomePaths() []string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if realHome, err := filepath.EvalSymlinks(home); err == nil {
			return []string{realHome}
		}
	}
	return nil
}
