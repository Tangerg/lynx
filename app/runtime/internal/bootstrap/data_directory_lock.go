package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrDataDirectoryInUse identifies an Open refusal caused by another Runtime
// instance owning the same canonical data directory.
var ErrDataDirectoryInUse = errors.New("runtime: data directory is already in use")

const dataDirectoryLockFile = ".runtime.lock"

type dataDirectoryLease struct {
	mu        sync.Mutex
	directory string
	file      *os.File
	released  bool
}

func acquireDataDirectoryLease(directory string) (*dataDirectoryLease, error) {
	if directory == "" {
		return nil, errors.New("runtime: data directory is required")
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("runtime: data directory must be absolute")
	}
	cleaned := filepath.Clean(directory)
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return nil, fmt.Errorf("runtime: create data directory %q: %w", cleaned, err)
	}
	canonical, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve data directory %q: %w", cleaned, err)
	}
	canonical = filepath.Clean(canonical)

	file, err := os.OpenFile(filepath.Join(canonical, dataDirectoryLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runtime: open data directory lock: %w", err)
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		if isLockContention(err) {
			return nil, fmt.Errorf("%w: %s", ErrDataDirectoryInUse, canonical)
		}
		return nil, fmt.Errorf("runtime: lock data directory %q: %w", canonical, err)
	}
	return &dataDirectoryLease{directory: canonical, file: file}, nil
}

func (l *dataDirectoryLease) release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	if err := unlockFile(l.file); err != nil {
		return fmt.Errorf("runtime: unlock data directory %q: %w", l.directory, err)
	}
	// Unlocking is the semantic release. Closing the regular lock-file handle
	// cannot restore ownership after a successful unlock, so it is deliberately
	// best-effort rather than reported as a retryable ownership failure.
	_ = l.file.Close()
	l.released = true
	return nil
}
