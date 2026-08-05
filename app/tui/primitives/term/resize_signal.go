//go:build unix

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyResize asks the operating system to report terminal size changes.
func notifyResize(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
