package runtimeownership

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/advisorylock"
)

// DataDirectorySetup serializes schema installation and configuration seeding.
// It is deliberately not a Runtime-lifetime lease: after setup, Runtime
// processes may share Directory.
type DataDirectorySetup struct {
	mu        sync.Mutex
	Directory string
	lease     *advisorylock.Lease
	released  bool
}

// PrepareDataDirectory canonicalizes and protects the shared data directory,
// then acquires its short setup lease.
func PrepareDataDirectory(ctx context.Context, directory string) (*DataDirectorySetup, error) {
	if directory == "" {
		return nil, errors.New("runtime ownership: data directory is required")
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("runtime ownership: data directory must be absolute")
	}
	cleaned := filepath.Clean(directory)
	if err := os.MkdirAll(cleaned, 0o700); err != nil {
		return nil, fmt.Errorf("runtime ownership: create data directory %q: %w", cleaned, err)
	}
	canonical, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return nil, fmt.Errorf("runtime ownership: resolve data directory %q: %w", cleaned, err)
	}
	canonical = filepath.Clean(canonical)
	if err := os.Chmod(canonical, 0o700); err != nil {
		return nil, fmt.Errorf("runtime ownership: protect data directory %q: %w", canonical, err)
	}
	lease, err := advisorylock.AcquireDirectory(ctx, canonical)
	if err != nil {
		return nil, fmt.Errorf("runtime ownership: serialize data directory setup %q: %w", canonical, err)
	}
	return &DataDirectorySetup{Directory: canonical, lease: lease}, nil
}

// Release ends the setup window. It is retryable and idempotent.
func (s *DataDirectorySetup) Release() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released {
		return nil
	}
	if err := s.lease.Release(); err != nil {
		return fmt.Errorf("runtime ownership: release data directory setup %q: %w", s.Directory, err)
	}
	s.released = true
	return nil
}
