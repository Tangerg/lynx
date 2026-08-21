package protocol

import (
	"go/ast"
	"go/token"
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

// featureConstants reads the values of every `Feature*` string constant in the
// package's non-test files.
func featureConstants(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, syntax := range parseProtocolSource(t) {
		for _, specification := range constantSpecs(syntax) {
			if text, ok := featureConstant(t, specification); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

func featureConstant(t *testing.T, specification *ast.ValueSpec) (string, bool) {
	t.Helper()
	if len(specification.Names) != 1 || len(specification.Values) != 1 ||
		!strings.HasPrefix(specification.Names[0].Name, "Feature") {
		return "", false
	}
	literal, ok := specification.Values[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatalf("unquote %s: %v", literal.Value, err)
	}
	return text, true
}
