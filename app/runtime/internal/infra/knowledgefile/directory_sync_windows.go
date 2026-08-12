//go:build windows

package knowledgefile

import (
	"os"

	"golang.org/x/sys/windows"
)

func syncCommittedDirectory(root *os.Root, path string) {
	directory, err := root.Open(path)
	if err != nil {
		return
	}
	_ = windows.FlushFileBuffers(windows.Handle(directory.Fd()))
	_ = directory.Close()
}
