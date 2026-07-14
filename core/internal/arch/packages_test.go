package arch

import (
	"path/filepath"
	"slices"
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
	"model":              {},
	"moderation":         {},
	"speech":             {},
	"transcription":      {},
	"vectorstore":        {},
	"vectorstore/filter": {},
}

var temporaryPublicPackages = map[string]string{
	"model/audio/transcription":     "P5-02",
	"model/audio/tts":               "P5-02",
	"model/chat":                    "P6-05",
	"model/chat/conversation":       "P3-03/P6-05",
	"model/chat/history":            "P3-04/P6-05",
	"model/chat/middleware/history": "P3-04/P6-05",
	"model/embedding":               "P5-01/P6-05",
	"model/image":                   "P5-02/P6-05",
	"model/moderation":              "P5-02/P6-05",
	"tokenizer":                     "P5-04",
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

	var temporary []string
	for packagePath := range seen {
		if _, ok := targetPublicPackages[packagePath]; ok {
			continue
		}
		deadline, ok := temporaryPublicPackages[packagePath]
		if !ok {
			t.Errorf("public package %q is outside the target architecture", packagePath)
			continue
		}
		temporary = append(temporary, packagePath+" -> "+deadline)
	}
	slices.Sort(temporary)
	for _, item := range temporary {
		t.Log("temporary public package remains: " + item)
	}
}
