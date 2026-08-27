package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/infra/gitprocess"
)

// git runs one git command against the shadow GIT_DIR with cwd as the work tree
// (workTree may be empty for repo-only operations like rev-parse). A fixed
// identity + disabled signing keep commits independent of the user's global git
// config.
func (s *Store) git(ctx context.Context, gitDir, workTree string, args ...string) (string, error) {
	output, err := s.gitOutput(ctx, gitDir, workTree, args...)
	return strings.TrimSpace(string(output)), err
}

func (s *Store) gitOutput(ctx context.Context, gitDir, workTree string, args ...string) ([]byte, error) {
	environment := []string{
		"GIT_DIR=" + gitDir,
		"GIT_AUTHOR_NAME=scopeapp", "GIT_AUTHOR_EMAIL=scopeapp@localhost",
		"GIT_COMMITTER_NAME=scopeapp", "GIT_COMMITTER_EMAIL=scopeapp@localhost",
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull,
		"LC_ALL=C", "LANG=C",
	}
	if workTree != "" {
		environment = append(environment, "GIT_WORK_TREE="+workTree)
	}
	result, err := gitprocess.Run(ctx, environment, args...)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: git %s: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return result.Stdout, &gitCommandError{
			operation: args[0], exitCode: result.ExitCode, diagnostic: strings.TrimSpace(result.Stderr),
		}
	}
	return result.Stdout, nil
}

type gitCommandError struct {
	operation  string
	exitCode   int
	diagnostic string
}

func (g *gitCommandError) Error() string {
	if g.diagnostic == "" {
		return fmt.Sprintf("checkpoint: git %s: exit code %d", g.operation, g.exitCode)
	}
	return fmt.Sprintf("checkpoint: git %s: %s: exit code %d", g.operation, g.diagnostic, g.exitCode)
}

func (g *gitCommandError) ExitCode() int { return g.exitCode }

func gitExitCode(err error) int {
	type exitCoder interface {
		error
		ExitCode() int
	}
	if exited, ok := errors.AsType[exitCoder](err); ok {
		return exited.ExitCode()
	}
	return -1
}

// gitIn runs a git query inside the real repo at cwd (no shadow GIT_DIR), used
// to discover what a new shadow repo can seed from. Returns trimmed stdout.
func gitIn(ctx context.Context, cwd string, args ...string) (string, error) {
	command := append([]string{"--no-pager", "--no-optional-locks", "-C", cwd}, args...)
	result, err := gitprocess.Run(ctx, []string{"LC_ALL=C", "LANG=C"}, command...)
	if err != nil {
		return "", fmt.Errorf("checkpoint: git %s: %w", args[0], err)
	}
	if result.ExitCode != 0 {
		return "", &gitCommandError{
			operation: args[0], exitCode: result.ExitCode, diagnostic: strings.TrimSpace(result.Stderr),
		}
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

func copyFile(src, dst string, maxBytes int64) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	written, err := io.CopyN(out, in, maxBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if written > maxBytes {
		return fmt.Errorf("%w: source index exceeds %d bytes", ErrSnapshotTooLarge, maxBytes)
	}
	return nil
}
