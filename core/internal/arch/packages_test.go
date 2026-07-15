package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

var targetPublicPackages = map[string]struct{}{
	"chat":               {},
	"document":           {},
	"embedding":          {},
	"image":              {},
	"media":              {},
	"metadata":           {},
	"moderation":         {},
	"speech":             {},
	"transcription":      {},
	"vectorstore":        {},
	"vectorstore/filter": {},
}

func TestPublicPackagesMatchArchitectureAllowlist(t *testing.T) {
	root := moduleRoot(t)
	seen := make(map[string]struct{})
	for _, path := range productionGoFiles(t) {
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatalf("relative package path for %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.Contains("/"+rel+"/", "/internal/") {
			continue
		}
		seen[rel] = struct{}{}
	}

	for packagePath := range seen {
		if _, ok := targetPublicPackages[packagePath]; ok {
			continue
		}
		t.Errorf("public package %q is outside the target architecture", packagePath)
	}
}
