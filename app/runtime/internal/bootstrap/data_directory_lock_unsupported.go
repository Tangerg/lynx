//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package bootstrap

import (
	"errors"
	"os"
)

var errDataDirectoryLockUnsupported = errors.New("runtime: data directory locking is unsupported on this platform")

func tryLockFile(*os.File) error  { return errDataDirectoryLockUnsupported }
func unlockFile(*os.File) error   { return nil }
func isLockContention(error) bool { return false }
