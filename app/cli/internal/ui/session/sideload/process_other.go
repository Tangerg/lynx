//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package sideload

import (
	"os/exec"
	"time"
)

func configureProcess(command *exec.Cmd) {
	command.WaitDelay = 250 * time.Millisecond
}
