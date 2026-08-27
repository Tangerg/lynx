package arch

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var retiredProductBrandTokens = [][]byte{
	[]byte("LY" + "RA"),
	[]byte("Ly" + "ra"),
	[]byte("ly" + "ra"),
}

var generatedProductBrandDirectories = []string{
	filepath.Join("desktop", "build", "bin"),
	filepath.Join("desktop", "build", "ios"),
	filepath.Join("desktop", "build", "linux"),
	filepath.Join("desktop", "build", "windows"),
}

var machineLocalRuntimeConfig = filepath.Join("runtime", "config", "config.yaml")

const (
	brandScanChunkBytes       = 64 << 10
	maxRetiredBrandTokenBytes = len("LY" + "RA")
)

// TestAppHasNoRetiredProductBrand keeps the breaking product-identity cutover
// complete. The frozen app/cli TUI is explicitly outside this app migration;
// generated dependencies and build outputs are not repository product sources.
func TestAppHasNoRetiredProductBrand(t *testing.T) {
	appRoot := filepath.Dir(moduleRoot(t))
	err := filepath.WalkDir(appRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(appRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if retiredBrandExcludedDirectory(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		if retiredBrandExcludedFile(relative) {
			return nil
		}
		for _, token := range retiredProductBrandTokens {
			if bytes.Contains([]byte(entry.Name()), token) {
				t.Errorf("retired product brand remains in path %s", relative)
			}
		}
		if !productBrandTextFile(entry.Name()) {
			return nil
		}
		retiredToken, err := retiredBrandInFile(path)
		if err != nil {
			return err
		}
		if retiredToken != "" {
			t.Errorf("retired product brand %q remains in %s", retiredToken, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan app product identity: %v", err)
	}
}

func retiredBrandInFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var window [brandScanChunkBytes + maxRetiredBrandTokenBytes - 1]byte
	overlap := 0
	for {
		n, readErr := file.Read(window[overlap:])
		chunk := window[:overlap+n]
		for _, token := range retiredProductBrandTokens {
			if bytes.Contains(chunk, token) {
				return string(token), nil
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return "", nil
			}
			return "", readErr
		}
		overlap = min(len(chunk), maxRetiredBrandTokenBytes-1)
		copy(window[:overlap], chunk[len(chunk)-overlap:])
	}
}

func retiredBrandExcludedDirectory(relative string) bool {
	if relative == "cli" || strings.HasPrefix(relative, "cli"+string(filepath.Separator)) {
		return true
	}
	base := filepath.Base(relative)
	if base == "node_modules" || base == "dist" {
		return true
	}
	for _, generated := range generatedProductBrandDirectories {
		if relative == generated {
			return true
		}
	}
	return false
}

func retiredBrandExcludedFile(relative string) bool {
	// Machine-local runtime configuration may contain API keys and provider-owned
	// identifiers. Its checked-in example remains part of the product scan.
	return relative == machineLocalRuntimeConfig
}

func productBrandTextFile(name string) bool {
	if name == "Makefile" || strings.HasPrefix(name, ".git") {
		return true
	}
	switch filepath.Ext(name) {
	case ".cjs", ".css", ".go", ".html", ".js", ".json", ".jsx", ".md", ".mjs",
		".plist", ".ts", ".tsx", ".txt", ".xml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
