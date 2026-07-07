package checkpoint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// git runs one git command against the shadow GIT_DIR with cwd as the work tree
// (workTree may be empty for repo-only operations like rev-parse). A fixed
// identity + disabled signing keep commits independent of the user's global git
// config.
func (s *Store) git(ctx context.Context, gitDir, workTree string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := append(os.Environ(),
		"GIT_DIR="+gitDir,
		"GIT_AUTHOR_NAME=lyra", "GIT_AUTHOR_EMAIL=lyra@localhost",
		"GIT_COMMITTER_NAME=lyra", "GIT_COMMITTER_EMAIL=lyra@localhost",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	if workTree != "" {
		env = append(env, "GIT_WORK_TREE="+workTree)
	}
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("checkpoint: git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// gitIn runs a git query inside the real repo at cwd (no shadow GIT_DIR), used
// to discover what a new shadow repo can seed from. Returns trimmed stdout.
func gitIn(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
