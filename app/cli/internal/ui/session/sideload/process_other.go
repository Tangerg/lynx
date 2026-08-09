//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package sideload

import (
	"os/exec"
	"time"
)

func configureProcess(command *exec.Cmd) {
	command.WaitDelay = 250 * time.Millisecond
}
