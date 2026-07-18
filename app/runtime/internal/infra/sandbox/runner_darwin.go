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
	if _, err := os.Stat(sandboxExecPath); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnavailable, sandboxExecPath, err)
	}
	paths := make([]string, 0, len(readOnlyPaths))
	for _, value := range readOnlyPaths {
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
	var hiddenPaths []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if realHome, err := filepath.EvalSymlinks(home); err == nil {
			hiddenPaths = append(hiddenPaths, realHome)
		}
	}
	return seatbeltRunner{readOnlyPaths: paths, hiddenPaths: hiddenPaths}, nil
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
	profile := r.profile(realDir)
	cmd := exec.CommandContext(runCtx, sandboxExecPath, "-p", profile, "/bin/sh", "-c", input.Cmd)
	cmd.Dir = realDir
	cmd.Env = []string{
		"HOME=" + realDir,
		"TMPDIR=" + realDir,
		"PATH=" + cmp.Or(os.Getenv("PATH"), "/usr/bin:/bin"),
		"LANG=" + cmp.Or(os.Getenv("LANG"), "C.UTF-8"),
	}
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

func (r seatbeltRunner) profile(dir string) string {
	var profile strings.Builder
	profile.WriteString("(version 1)\n(deny default)\n")
	profile.WriteString("(allow process*)\n(allow signal (target self))\n")
	profile.WriteString("(allow sysctl-read)\n(allow mach-lookup)\n")
	// macOS command startup reads a broad and OS-version-dependent set of
	// system resources. Permit host reads, then carve out the user's home and
	// selectively re-open only declared toolchain roots plus this workspace.
	// Writes remain workspace-only, and the process receives a scrubbed env.
	profile.WriteString("(allow file-read*)\n")
	for _, value := range r.hiddenPaths {
		profile.WriteString("(deny file-read* (subpath ")
		profile.WriteString(strconv.Quote(value))
		profile.WriteString(") )\n")
	}
	for _, value := range r.readOnlyPaths {
		profile.WriteString("(allow file-read* (subpath ")
		profile.WriteString(strconv.Quote(value))
		profile.WriteString(") )\n")
	}
	profile.WriteString("(allow file-read* file-write* (subpath ")
	profile.WriteString(strconv.Quote(dir))
	profile.WriteString(") )\n")
	return profile.String()
}
