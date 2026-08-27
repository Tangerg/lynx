//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package advisorylock

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func tryFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, unix.LOCK_EX)
}

func trySharedFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, unix.LOCK_SH)
}

func tryFileMode(file *os.File, mode int) (*Lease, error) {
	if err := unix.Flock(int(file.Fd()), mode|unix.LOCK_NB); err != nil {
		if isContention(err) {
			return nil, ErrContended
		}
		return nil, err
	}
	return newLease(func() error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }), nil
}

func acquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	file, err := os.Open(directory)
	if err != nil {
		return nil, err
	}
	retry := time.NewTicker(time.Millisecond)
	defer retry.Stop()
	for {
		if cause := context.Cause(ctx); cause != nil {
			_ = file.Close()
			return nil, cause
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return newLease(func() error {
				if unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN); unlockErr != nil {
					return unlockErr
				}
				_ = file.Close()
				return nil
			}), nil
		}
		if !isContention(err) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, context.Cause(ctx)
		case <-retry.C:
		}
	}
}

func tryDirectory(directory string) (*Lease, error) {
	file, err := os.Open(directory)
	if err != nil {
		return nil, err
	}
	lease, err := tryFile(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return newLease(func() error {
		if err := lease.Release(); err != nil {
			return err
		}
		_ = file.Close()
		return nil
	}), nil
}

func isContention(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
