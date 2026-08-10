//go:build windows

package sideload

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	command.Cancel = func() error {
		// taskkill /T terminates descendants as well as the manifest entry. Fall
		// back to the root process when taskkill is unavailable or races exit.
		pid := strconv.Itoa(command.Process.Pid)
		if err := exec.Command("taskkill", "/T", "/F", "/PID", pid).Run(); err == nil {
			return nil
		}
		err := command.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 250 * time.Millisecond
}
