//go:build windows

package hookprocess

import (
	"context"
	"os"
	"os/exec"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
}

func prepareProcessGroup(*exec.Cmd) {}

func stopProcessGroup(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
