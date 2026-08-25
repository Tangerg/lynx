//go:build windows

package exec

import (
	"os"
	"os/exec"
)

func configureShellProcess(*exec.Cmd) {}

func stopShellProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
