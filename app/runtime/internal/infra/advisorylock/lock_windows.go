//go:build windows

package advisorylock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func tryFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, true)
}

func trySharedFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, false)
}

func tryFileMode(file *os.File, exclusive bool) (*Lease, error) {
	var overlapped windows.Overlapped
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		flags,
		0, 1, 0, &overlapped,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, ErrContended
		}
		return nil, err
	}
	return newLease(func() error {
		var overlapped windows.Overlapped
		return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	}), nil
}

func acquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	return directoryLease(ctx, directory, false)
}

func tryDirectory(directory string) (*Lease, error) {
	return directoryLease(context.Background(), directory, true)
}

func directoryLease(ctx context.Context, directory string, nonblocking bool) (*Lease, error) {
	type result struct {
		lease *Lease
		err   error
	}
	resultChannel := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle, err := acquireDirectoryHandle(ctx, directory, nonblocking)
		if err != nil {
			resultChannel <- result{err: err}
			return
		}
		requests := make(chan chan error)
		lease := newLease(func() error {
			response := make(chan error, 1)
			requests <- response
			return <-response
		})
		resultChannel <- result{lease: lease}
		for response := range requests {
			err := windows.ReleaseMutex(handle)
			response <- err
			if err == nil {
				_ = windows.CloseHandle(handle)
				return
			}
		}
	}()
	acquired := <-resultChannel
	return acquired.lease, acquired.err
}

func acquireDirectoryHandle(ctx context.Context, directory string, nonblocking bool) (windows.Handle, error) {
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return 0, err
	}
	var identity windows.ByHandleFileInformation
	identityErr := windows.GetFileInformationByHandle(windows.Handle(directoryHandle.Fd()), &identity)
	closeErr := directoryHandle.Close()
	if err := errors.Join(identityErr, closeErr); err != nil {
		return 0, err
	}
	name := fmt.Sprintf(
		`Local\Lyra-Advisory-Directory-%08x-%08x%08x`,
		identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow,
	)
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateMutex(nil, false, nameUTF16)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return 0, err
	}
	for {
		if !nonblocking {
			if err := context.Cause(ctx); err != nil {
				_ = windows.CloseHandle(handle)
				return 0, err
			}
		}
		result, err := windows.WaitForSingleObject(handle, 10)
		if err != nil {
			_ = windows.CloseHandle(handle)
			return 0, err
		}
		switch result {
		case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
			return handle, nil
		case uint32(windows.WAIT_TIMEOUT):
			if nonblocking {
				_ = windows.CloseHandle(handle)
				return 0, ErrContended
			}
			select {
			case <-ctx.Done():
				_ = windows.CloseHandle(handle)
				return 0, context.Cause(ctx)
			default:
			}
		default:
			_ = windows.CloseHandle(handle)
			return 0, fmt.Errorf("advisory lock: unexpected wait result %d", result)
		}
	}
}
