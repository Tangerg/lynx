// Package storage resolves Lyra's on-disk root ([Home], usually ~/.lyra or
// $LYRA_HOME) and provides the file-backed knowledge store (the editable LYRA.md
// cascade). Database-backed state lives in the sqlite sub-package.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Home returns the root directory Lyra reads and writes state under.
// Resolution order: the LYRA_HOME environment variable, else
// ~/.lyra. The directory is created if missing.
//
// Errors when neither LYRA_HOME nor a usable home directory can be
// determined.
func Home() (string, error) {
	if v := os.Getenv("LYRA_HOME"); v != "" {
		if err := os.MkdirAll(v, 0o755); err != nil {
			return "", fmt.Errorf("storage: create LYRA_HOME %q: %w", v, err)
		}
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("storage: locate user home: %w", err)
	}
	root := filepath.Join(home, ".lyra")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("storage: create %q: %w", root, err)
	}
	return root, nil
}
