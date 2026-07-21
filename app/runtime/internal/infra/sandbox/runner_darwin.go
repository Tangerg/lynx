//go:build darwin

package sandbox

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	toolshell "github.com/Tangerg/lynx/tools/shell"
)

const sandboxExecPath = "/usr/bin/sandbox-exec"

type seatbeltRunner struct {
	readOnlyPaths []string
	hiddenPaths   []string
}

func platformRunner(readOnlyPaths []string) (commandRunner, error) {
	if err := checkSandboxExec(); err != nil {
		return nil, err
	}
	paths, err := resolveReadOnlyPaths(readOnlyPaths)
	if err != nil {
		return nil, err
	}
	return seatbeltRunner{readOnlyPaths: paths, hiddenPaths: hiddenHomePaths()}, nil
}

// ConfineShellCommand rewrites a shell command into the argv and scrubbed
// environment that run it under a Seatbelt jail confining writes to
// writableRoot: writableRoot is the only writable subtree, all networking is
// denied, the real $HOME is hidden (except any readOnlyPaths re-opened for
// reads), and the process runs with a minimal environment (HOME/TMPDIR pinned
// to writableRoot). The returned name/args build the exec.Cmd; the caller still
// owns process lifecycle (start, wait, cancel, streaming) and sets cmd.Dir to
// writableRoot itself.
//
// It is the in-place counterpart to [Workspace], which runs commands inside an
// isolated copy: a jail leaves the caller's working tree in place. It fails
// closed with [ErrUnavailable] when the host has no supported backend (only
// macOS Seatbelt exists today — see runner_other.go).
func ConfineShellCommand(writableRoot string, readOnlyPaths []string, command string) (name string, args []string, env []string, err error) {
	if command == "" {
		return "", nil, nil, errors.New("sandbox: command is required")
	}
	if err := checkSandboxExec(); err != nil {
		return "", nil, nil, err
	}
	realRoot, err := filepath.EvalSymlinks(writableRoot)
	if err != nil {
		return "", nil, nil, fmt.Errorf("sandbox: resolve workspace: %w", err)
	}
	readable, err := resolveReadOnlyPaths(readOnlyPaths)
	if err != nil {
		return "", nil, nil, err
	}
	profile := seatbeltProfile(realRoot, readable, hiddenHomePaths())
	return sandboxExecPath, []string{"-p", profile, "/bin/sh", "-c", command}, scrubbedEnv(realRoot), nil
}

func checkSandboxExec() error {
	if _, err := os.Stat(sandboxExecPath); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, sandboxExecPath, err)
	}
	return nil
}

// resolveReadOnlyPaths canonicalizes the caller-declared read-only roots
// (absolute + symlink-resolved) that the jail re-opens for reads below the
// hidden home. A non-resolvable entry is an error — an unreachable path in a
// security policy is a configuration mistake, not something to silently drop.
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
// or nil when it can't be resolved (non-fatal: the env is scrubbed regardless).
func hiddenHomePaths() []string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if realHome, err := filepath.EvalSymlinks(home); err == nil {
			return []string{realHome}
		}
	}
	return nil
}

// scrubbedEnv is the minimal environment a jailed command runs with. HOME and
// TMPDIR are pinned to the writable root so tools that key off them land inside
// the jail; every other parent variable (credentials, tokens) is dropped.
func scrubbedEnv(root string) []string {
	return []string{
		"HOME=" + root,
		"TMPDIR=" + root,
		"PATH=" + cmp.Or(os.Getenv("PATH"), "/usr/bin:/bin"),
		"LANG=" + cmp.Or(os.Getenv("LANG"), "C.UTF-8"),
	}
}

func (r seatbeltRunner) Run(ctx context.Context, dir string, input toolshell.Input) (toolshell.Output, error) {
	if input.Cmd == "" {
		return toolshell.Output{}, errors.New("sandbox: command is required")
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return toolshell.Output{}, fmt.Errorf("sandbox: resolve workspace: %w", err)
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if input.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, input.Timeout)
		defer cancel()
	}
	profile := seatbeltProfile(realDir, r.readOnlyPaths, r.hiddenPaths)
	cmd := exec.CommandContext(runCtx, sandboxExecPath, "-p", profile, "/bin/sh", "-c", input.Cmd)
	cmd.Dir = realDir
	cmd.Env = scrubbedEnv(realDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err = cmd.Run()
	cleanupErr := killProcessGroup(cmd)
	out := toolshell.Output{
		Stdout:   stdout.BytesWithMarker(),
		Stderr:   stderr.BytesWithMarker(),
		Duration: time.Since(started),
		Killed:   runCtx.Err() != nil,
	}
	if cleanupErr != nil {
		return out, cleanupErr
	}
	if err == nil {
		return out, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	}
	return out, fmt.Errorf("sandbox: start command: %w", err)
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("sandbox: stop command descendants: %w", err)
}

// seatbeltProfile builds the SBPL policy confining writes to writableRoot.
// Later rules win in SBPL, so the ordering is deliberate: permit host reads,
// deny the user's home, re-open declared toolchain roots, then grant read+write
// to the workspace last — a workspace nested under home is therefore re-opened
// read+write by the final rule. Network is denied by omission (deny default +
// no network rule). Writes remain workspace-only.
func seatbeltProfile(writableRoot string, readOnlyPaths, hiddenPaths []string) string {
	var profile strings.Builder
	profile.WriteString("(version 1)\n(deny default)\n")
	profile.WriteString("(allow process*)\n(allow signal (target self))\n")
	profile.WriteString("(allow sysctl-read)\n(allow mach-lookup)\n")
	profile.WriteString("(allow file-read*)\n")
	for _, value := range hiddenPaths {
		profile.WriteString("(deny file-read* (subpath ")
		profile.WriteString(strconv.Quote(value))
		profile.WriteString(") )\n")
	}
	for _, value := range readOnlyPaths {
		profile.WriteString("(allow file-read* (subpath ")
		profile.WriteString(strconv.Quote(value))
		profile.WriteString(") )\n")
	}
	profile.WriteString("(allow file-read* file-write* (subpath ")
	profile.WriteString(strconv.Quote(writableRoot))
	profile.WriteString(") )\n")
	return profile.String()
}
