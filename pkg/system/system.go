package system

import (
	"runtime"
)

var lineSeparator string

func init() {
	if runtime.GOOS == "windows" {
		lineSeparator = "\r\n"
	} else {
		lineSeparator = "\n"
	}
}

/*
LineSeparator
Returns the system-dependent line separator string. It always returns the same value - the initial value of the system property line. separator.
On UNIX systems, it returns "\n"; on Microsoft Windows systems it returns "\r\n".
Returns:
the system-dependent line separator string
*/
func LineSeparator() string {
	return lineSeparator
}
