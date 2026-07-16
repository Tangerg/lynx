package arch

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

var updateAgentWireFixtures = flag.Bool(
	"update-wire-fixtures",
	false,
	"replace the reviewed Agent wire contract fixture",
)

// agentWireFixtureCoverage maps every exported JSON-tagged struct in the
// durable core/interaction/toolloop protocol packages to the representative
// root that exercises it.
var agentWireFixtureCoverage = map[string]string{
	"core.DeploymentRef":     "process_snapshot",
	"core.EmbeddingCall":     "process_snapshot",
	"core.ModelCall":         "process_snapshot",
	"core.ProcessSnapshot":   "process_snapshot",
	"core.Session":           "session",
	"core.ActionRunSnapshot": "process_snapshot",
	"core.TaggedValue":       "process_snapshot",
	"interaction.Event":      "interaction_events",
	"interaction.Resume":     "interaction_events",
	"interaction.Suspension": "process_snapshot",
	"toolloop.Checkpoint":    "toolloop_checkpoint",
	"toolloop.Event":         "toolloop_events",
	"toolloop.Pause":         "toolloop_events",
	"toolloop.Resume":        "toolloop_events",
}

var agentWirePackages = map[string]struct{}{
	"core":        {},
	"interaction": {},
	"toolloop":    {},
}

func TestWireTypeCoverage(t *testing.T) {
	t.Parallel()

	got := discoverAgentExportedJSONStructs(t)
	want := make([]string, 0, len(agentWireFixtureCoverage))
	for name := range agentWireFixtureCoverage {
		want = append(want, name)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Agent exported JSON struct inventory changed\nwant: %v\n got: %v\nclassify the wire change, then update agentWireFixtureCoverage and wire_contracts.golden.json", want, got)
	}
}

func TestWireContractsMatchGolden(t *testing.T) {
	got, err := json.MarshalIndent(representativeAgentWireContracts(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	filename := filepath.Join(moduleRoot(t), "internal", "arch", "testdata", "wire_contracts.golden.json")
	if *updateAgentWireFixtures {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Agent wire contract changed\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func discoverAgentExportedJSONStructs(t *testing.T) []string {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()
	var names []string
	for _, filename := range productionGoFiles(t) {
		packagePath, err := filepath.Rel(root, filepath.Dir(filename))
		if err != nil {
			t.Fatal(err)
		}
		packagePath = filepath.ToSlash(packagePath)
		if _, tracked := agentWirePackages[packagePath]; !tracked {
			continue
		}

		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !ast.IsExported(typeSpec.Name.Name) || !agentHasJSONTag(structure) {
					continue
				}
				names = append(names, file.Name.Name+"."+typeSpec.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return names
}

func agentHasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err == nil && reflect.StructTag(tag).Get("json") != "" {
			return true
		}
	}
	return false
}
