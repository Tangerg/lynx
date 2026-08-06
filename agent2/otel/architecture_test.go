package otel

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOTelAdapterExcludesStrategiesLegacyAndHostPackages(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "github.com/Tangerg/lynx/agent" || strings.HasPrefix(path, "github.com/Tangerg/lynx/agent/") ||
				path == "github.com/Tangerg/lynx/app" || strings.HasPrefix(path, "github.com/Tangerg/lynx/app/") ||
				strings.HasPrefix(path, "github.com/Tangerg/lynx/agent2/interaction") ||
				strings.HasPrefix(path, "github.com/Tangerg/lynx/agent2/planning") ||
				strings.HasPrefix(path, "github.com/Tangerg/lynx/agent2/workflow") ||
				path == "go.opentelemetry.io/otel/sdk" || strings.HasPrefix(path, "go.opentelemetry.io/otel/sdk/") {
				t.Errorf("%s imports forbidden Strategy, legacy, Host, or OTel SDK package %q", name, path)
			}
		}
	}
}
