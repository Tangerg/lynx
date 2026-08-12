//go:build !windows

package knowledgefile

import (
	"errors"
	"os"
)

func syncCommittedDirectory(root *os.Root, path string) {
	directory, err := root.Open(path)
	if err != nil {
		return
	}
	_ = errors.Join(directory.Sync(), directory.Close())
}
