package protocol

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestFeaturesAreComplete checks [Features] against the constants this package
// declares, in both directions.
//
// Same reasoning as [TestWireEnumsAreComplete]: a Go const block cannot be
// enumerated at runtime, so the published vocabulary has to be written down twice —
// once as constants a rule can reference, once as a list the generator can walk.
// A declared constant missing from the list is a capability clients can never
// name; a listed name with no constant is one the runtime will never advertise.
func TestFeaturesAreComplete(t *testing.T) {
	declared := featureConstants(t)
	published := FeatureKeys()

	for _, feature := range declared {
		if !slices.Contains(published, feature) {
			t.Errorf("a Feature constant declares %q and Features does not list it", feature)
		}
	}
	for _, feature := range published {
		if !slices.Contains(declared, feature) {
			t.Errorf("Features lists %q and no Feature constant declares it", feature)
		}
	}
}

func TestFeaturesReturnsASnapshot(t *testing.T) {
	t.Parallel()

	first := Features()
	original := first[0]
	first[0].Key = "corrupted"
	if got := Features()[0]; got != original {
		t.Fatalf("Features exposed registry storage: got %+v, want %+v", got, original)
	}
}

func TestFeatureRegistryRejectsUnknownStability(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("mustFeatures accepted an unknown stability")
		}
	}()
	mustFeatures([]Feature{{
		Key: "test", Stability: Stability("accidental"),
	}})
}

// featureConstants reads the values of every `Feature*` string constant in the
// package's non-test files.
func featureConstants(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package files: %v", err)
	}
	slices.Sort(files)

	var out []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range syntax.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, spec := range group.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if !strings.HasPrefix(value.Names[0].Name, "Feature") {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", literal.Value, err)
				}
				out = append(out, text)
			}
		}
	}
	return out
}
