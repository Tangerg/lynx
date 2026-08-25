//go:build unix

package mcp

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// prepareStdioProcess gives each MCP stdio server a process group owned by its
// session. CommandContext cancellation must terminate that group as well: MCP
// servers commonly launch package runners or language-specific child servers.
func prepareStdioProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return stopStdioProcess(command) }
}

func stopStdioProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
