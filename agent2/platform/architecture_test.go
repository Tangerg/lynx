package platform

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
)

func TestDeploymentCandidateContainsOnlyDiscoveryContracts(t *testing.T) {
	typeOf := reflect.TypeFor[DeploymentCandidate]()
	if typeOf.NumField() != 2 || typeOf.Field(0).Name != "reference" ||
		typeOf.Field(0).Type != reflect.TypeFor[agent.DeploymentRef]() ||
		typeOf.Field(1).Name != "descriptor" ||
		typeOf.Field(1).Type != reflect.TypeFor[agent.Descriptor]() {
		t.Fatalf("DeploymentCandidate fields changed: %v", typeOf)
	}
	for index := range typeOf.NumField() {
		if typeOf.Field(index).IsExported() {
			t.Fatalf("DeploymentCandidate exposes mutable field %s", typeOf.Field(index).Name)
		}
	}
}

func TestPlatformExcludesStrategiesLegacyAndHostPackages(t *testing.T) {
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
				strings.HasPrefix(path, "go.opentelemetry.io/otel") {
				t.Errorf("%s imports forbidden Strategy, legacy, Host, or OTel package %q", name, path)
			}
		}
	}
}
