package system

import "runtime"

// lineSep holds the OS-specific line separator computed at init time.
var lineSep = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

// LineSeparator returns the OS line separator: "\r\n" on Windows and
// "\n" elsewhere. The value is fixed for the lifetime of the process.
func LineSeparator() string {
	return lineSep
}
