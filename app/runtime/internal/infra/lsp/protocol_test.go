package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseLocationsSupportsProtocolUnion(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []Location
	}{
		{name: "null", raw: "null"},
		{
			name: "single location",
			raw:  `{"uri":"file:///single.go","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":3}}}`,
			want: []Location{{URI: "file:///single.go", Range: Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 1, Character: 3}}}},
		},
		{
			name: "location array",
			raw:  `[{"uri":"file:///many.go","range":{"start":{"line":2,"character":3},"end":{"line":2,"character":4}}}]`,
			want: []Location{{URI: "file:///many.go", Range: Range{Start: Position{Line: 2, Character: 3}, End: Position{Line: 2, Character: 4}}}},
		},
		{
			name: "location link array",
			raw:  `[{"targetUri":"file:///link.go","targetRange":{"start":{"line":3,"character":0},"end":{"line":4,"character":0}},"targetSelectionRange":{"start":{"line":3,"character":5},"end":{"line":3,"character":9}}}]`,
			want: []Location{{URI: "file:///link.go", Range: Range{Start: Position{Line: 3, Character: 5}, End: Position{Line: 3, Character: 9}}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLocations(json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("parseLocations: %v", err)
			}
			if !locationsEqual(got, test.want) {
				t.Fatalf("locations = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseLocationsRejectsMalformedAndUnknownShapes(t *testing.T) {
	for _, raw := range []string{
		`false`,
		`{"range":{}}`,
		`[{"uri":""}]`,
		`[{"uri":"file:///a.go","targetUri":"file:///b.go"}]`,
		`[{]`,
	} {
		if _, err := parseLocations(json.RawMessage(raw)); err == nil {
			t.Errorf("parseLocations(%q) succeeded, want error", raw)
		}
	}
}

func TestParseSymbolsSupportsFlatAndHierarchicalShapes(t *testing.T) {
	flat, err := parseSymbols(json.RawMessage(`[{"name":"Run","kind":12,"location":{"uri":"file:///flat.go","range":{}}}]`), "file:///ignored.go")
	if err != nil {
		t.Fatalf("parse flat symbols: %v", err)
	}
	if len(flat) != 1 || flat[0].Name != "Run" || flat[0].Location.URI != "file:///flat.go" {
		t.Fatalf("flat symbols = %+v", flat)
	}

	tree, err := parseSymbols(json.RawMessage(`[{"name":"Service","kind":5,"range":{},"selectionRange":{},"children":[{"name":"Run","kind":6,"range":{},"selectionRange":{}}]}]`), "file:///tree.go")
	if err != nil {
		t.Fatalf("parse hierarchical symbols: %v", err)
	}
	if len(tree) != 2 || tree[1].Name != "Run" || tree[1].Container != "Service" || tree[1].Location.URI != "file:///tree.go" {
		t.Fatalf("hierarchical symbols = %+v", tree)
	}
}

func TestParseSymbolsRejectsMalformedAndAmbiguousShapes(t *testing.T) {
	for _, test := range []struct {
		raw    string
		docURI string
	}{
		{raw: `{}`, docURI: "file:///doc.go"},
		{raw: `[{"name":"","kind":12,"location":{"uri":"file:///doc.go","range":{}}}]`, docURI: "file:///doc.go"},
		{raw: `[{"name":"Run","kind":12,"location":{"uri":"","range":{}}}]`, docURI: "file:///doc.go"},
		{raw: `[{"name":"Run","kind":12,"range":{},"selectionRange":{}}]`},
		{raw: `[{]`, docURI: "file:///doc.go"},
	} {
		if _, err := parseSymbols(json.RawMessage(test.raw), test.docURI); err == nil {
			t.Errorf("parseSymbols(%q) succeeded, want error", test.raw)
		}
	}
}

func TestHoverTextSupportsProtocolUnion(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: `" value "`, want: "value"},
		{raw: `{"kind":"markdown","value":" **value** "}`, want: "**value**"},
		{raw: `[{"language":"go","value":"func Run()"},"details"]`, want: "func Run()\n\ndetails"},
	}
	for _, test := range tests {
		got, err := hoverText(json.RawMessage(test.raw))
		if err != nil {
			t.Fatalf("hoverText(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Errorf("hoverText(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestParseHoverHandlesNullAndContents(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "null"},
		{raw: `{"contents":{"kind":"markdown","value":" **value** "}}`, want: "**value**"},
	} {
		got, err := parseHover(json.RawMessage(test.raw))
		if err != nil {
			t.Fatalf("parseHover(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Errorf("parseHover(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
	for _, raw := range []string{"", `{}`, `{"contents":null}`, `[]`} {
		if _, err := parseHover(json.RawMessage(raw)); err == nil {
			t.Errorf("parseHover(%q) succeeded, want error", raw)
		}
	}
}

func TestHoverTextRejectsMalformedAndUnknownShapes(t *testing.T) {
	for _, raw := range []string{`null`, `true`, `{}`, `{"value":null}`, `[false]`, `{"value":`} {
		_, err := hoverText(json.RawMessage(raw))
		if err == nil {
			t.Errorf("hoverText(%q) succeeded, want error", raw)
		}
	}
}

func locationsEqual(left, right []Location) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestDiagnosticSeverityClosesProtocolVocabulary(t *testing.T) {
	if !DiagnosticSeverityUnspecified.IsProblem() || !DiagnosticSeverityWarning.IsProblem() {
		t.Fatal("unspecified and warning severities must be treated as problems")
	}
	if DiagnosticSeverityInformation.IsProblem() || DiagnosticSeverityHint.IsProblem() {
		t.Fatal("information and hint severities must not be treated as problems")
	}
	if got := (Diagnostic{Severity: DiagnosticSeverityError}).SeverityName(); got != "error" {
		t.Fatalf("error severity name = %q", got)
	}
	if got := (Diagnostic{Severity: 99}).SeverityName(); strings.TrimSpace(got) != "" {
		t.Fatalf("unknown severity name = %q, want empty", got)
	}
}
