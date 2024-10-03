package system

import (
	"runtime"
)

/*
LineSeparator
Returns the system-dependent line separator string. It always returns the same value - the initial value of the system property line. separator.
On UNIX systems, it returns "\n"; on Microsoft Windows systems it returns "\r\n".
Returns:
the system-dependent line separator string
*/
func LineSeparator() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}
