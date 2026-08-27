// Package advisorylock provides process-scoped filesystem leases for Infra
// components that need one cross-process linearization point.
package advisorylock

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/Tangerg/scope/app/runtime/internal/infra/pathidentity"
)

var (
	// ErrContended reports that a non-blocking acquisition found another owner.
	ErrContended = errors.New("advisory lock: already owned")
	// ErrUnsupported reports a platform without the required locking primitive.
	ErrUnsupported = errors.New("advisory lock: unsupported platform")
)

// Lease is one acquired advisory lock. Release is retryable and idempotent.
type Lease struct {
	mu       sync.Mutex
	release  func() error
	released bool
}

func newLease(release func() error) *Lease {
	return &Lease{release: release}
}

// Release relinquishes the lease. A failed release remains retryable.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	if err := l.release(); err != nil {
		return err
	}
	l.released = true
	return nil
}

// TryFile exclusively locks the first byte of an already-open regular file.
// The caller retains ownership of the file descriptor and closes it after the
// lease is released.
func TryFile(file *os.File) (*Lease, error) {
	if file == nil {
		return nil, errors.New("advisory lock: file is required")
	}
	return tryFile(file)
}

// TrySharedFile shared-locks the first byte of an already-open regular file.
// Multiple shared holders may coexist, while an exclusive [TryFile] holder
// excludes them. The caller retains ownership of the file descriptor and
// closes it after the lease is released.
func TrySharedFile(file *os.File) (*Lease, error) {
	if file == nil {
		return nil, errors.New("advisory lock: file is required")
	}
	return trySharedFile(file)
}

// TryDirectory acquires a non-blocking exclusive lease for the physical
// directory identity. It creates no files in the target tree.
func TryDirectory(directory string) (*Lease, error) {
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return tryDirectory(physical)
}

// AcquireDirectory waits until the physical directory identity is exclusively
// leased or ctx ends. Directory leases create no files in the target tree.
func AcquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	if ctx == nil {
		return nil, errors.New("advisory lock: context is required")
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return acquireDirectory(ctx, physical)
}

func resolveDirectory(directory string) (string, error) {
	physical, err := pathidentity.Resolve("", directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(physical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("advisory lock: identity is not a directory")
	}
	return physical, nil
}
