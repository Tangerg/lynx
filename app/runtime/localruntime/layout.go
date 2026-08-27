package localruntime

import (
	"errors"
	"fmt"
	"path/filepath"
)

const (
	dataDirectoryName = ".scopeapp"
	databaseFilename  = "scopeapp.db"
	localTokenName    = "local-token"
)

// ErrInvalidDataDirectory identifies a path that cannot own local Runtime
// durability. Callers only receive a DataDirectory after its absolute-path
// invariant has been established.
var ErrInvalidDataDirectory = errors.New("invalid local Runtime data directory")

// DataDirectory is the canonical local deployment root shared by the Runtime
// process and trusted desktop clients. It owns the filenames that cross that
// process boundary so consumers cannot construct competing layouts.
type DataDirectory struct {
	path string
}

// DataDirectoryAt validates an explicitly configured deployment root.
func DataDirectoryAt(path string) (DataDirectory, error) {
	if path == "" {
		return DataDirectory{}, invalidDataDirectory("path is required")
	}
	if !filepath.IsAbs(path) {
		return DataDirectory{}, invalidDataDirectory("path must be absolute")
	}
	return DataDirectory{path: filepath.Clean(path)}, nil
}

// DefaultDataDirectory derives the product-owned deployment root beneath an
// absolute user home directory.
func DefaultDataDirectory(userHome string) (DataDirectory, error) {
	if userHome == "" {
		return DataDirectory{}, invalidDataDirectory("user home is required")
	}
	if !filepath.IsAbs(userHome) {
		return DataDirectory{}, invalidDataDirectory("user home must be absolute")
	}
	return DataDirectoryAt(filepath.Join(filepath.Clean(userHome), dataDirectoryName))
}

// Path returns the absolute deployment root, or an empty string for the invalid
// zero value.
func (d DataDirectory) Path() string { return d.path }

// DatabasePath returns the Runtime database owned by this deployment root.
func (d DataDirectory) DatabasePath() string {
	return d.join(databaseFilename)
}

// LocalTokenPath returns the durable credential shared with trusted local
// clients.
func (d DataDirectory) LocalTokenPath() string {
	return d.join(localTokenName)
}

func (d DataDirectory) join(name string) string {
	if d.path == "" {
		return ""
	}
	return filepath.Join(d.path, name)
}

func invalidDataDirectory(reason string) error {
	return fmt.Errorf("local Runtime data directory: %s: %w", reason, ErrInvalidDataDirectory)
}
