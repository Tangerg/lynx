// Package storage implements Lyra's on-disk persistence: session
// metadata and chat-memory messages. Every
// concrete type targets a directory rooted at [Home] — usually
// ~/.lyra — so a single LYRA_HOME environment variable relocates
// every artifact.
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
