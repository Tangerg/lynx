//go:build windows

package shellflow

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	process := exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	process.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	process.Cancel = func() error {
		if process.Process == nil {
			return os.ErrProcessDone
		}
		pid := strconv.Itoa(process.Process.Pid)
		if err := exec.Command("taskkill", "/T", "/F", "/PID", pid).Run(); err == nil {
			return nil
		}
		return process.Process.Kill()
	}
	return process
}
