//go:build windows

package hooks

import (
	"context"
	"os"
	"os/exec"
)

func hookShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
}

func prepareHookProcessGroup(*exec.Cmd) {}

func stopHookProcessGroup(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
