//go:build windows

package mcp

import (
	"os"
	"os/exec"
)

func prepareStdioProcess(*exec.Cmd) {}

func stopStdioProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
