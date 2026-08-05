//go:build !unix

package term

import "os"

// notifyResize does nothing where the operating system has no resize signal.
//
// Windows reports console resizing through its own input API rather than through a
// signal. A session there still gets its opening size event, and a host that owns
// the console can deliver later sizes itself.
func notifyResize(chan<- os.Signal) {}
