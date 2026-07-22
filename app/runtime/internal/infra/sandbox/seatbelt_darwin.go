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

// checkBackend reports whether this host has a working isolation backend. macOS
// Seatbelt is the only one today; its absence is the fail-closed signal.
func checkBackend() error {
	if _, err := os.Stat(sandboxExecPath); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, sandboxExecPath, err)
	}
	return nil
}

// seatbeltCommand assembles the confined [Command] that runs command under a
// Seatbelt jail rooted at root: root is the only writable subtree, the reads are
// governed by the profile, and the environment is scrubbed. It is the single
// confinement core shared by the in-place [Confiner] and the isolated-copy
// [Workspace] runner. root and command are pre-validated by the caller.
func seatbeltCommand(root, command string, readOnly, hidden []string) Command {
	return Command{
		Name: sandboxExecPath,
		Args: []string{"-p", seatbeltProfile(root, readOnly, hidden), "/bin/sh", "-c", command},
		Env:  scrubbedEnv(root),
	}
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

// seatbeltRunner runs a command to completion, buffered, inside an isolated
// [Workspace] copy. It shares [seatbeltCommand] with the in-place [Confiner]:
// one confinement policy, two execution styles.
type seatbeltRunner struct {
	readOnlyPaths []string
	hiddenPaths   []string
}

func platformRunner(readOnlyPaths []string) (commandRunner, error) {
	if err := checkBackend(); err != nil {
		return nil, err
	}
	paths, err := resolveReadOnlyPaths(readOnlyPaths)
	if err != nil {
		return nil, err
	}
	return seatbeltRunner{readOnlyPaths: paths, hiddenPaths: hiddenHomePaths()}, nil
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
	confined := seatbeltCommand(realDir, input.Cmd, r.readOnlyPaths, r.hiddenPaths)
	cmd := exec.CommandContext(runCtx, confined.Name, confined.Args...)
	cmd.Dir = realDir
	cmd.Env = confined.Env
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
