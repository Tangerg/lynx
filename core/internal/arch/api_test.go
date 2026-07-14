package arch

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateAPI = flag.Bool("update-api", false, "replace the reviewed Core exported API baseline")

const exportedAPIBaseline = "testdata/exported_api.txt"

// TestExportedAPIMatchesBaseline is the release guard for Core's public Go
// surface. It snapshots every declaration that exports a package object plus
// every exported method signature. Any difference must be reviewed as an API
// decision, documented, and accepted by updating the checked-in baseline with:
//
//	go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api
//
// The snapshot is deliberately conservative: a const/var declaration group is
// kept whole when it contains an exported name, so iota ordering and inferred
// types cannot change without review. Function bodies and comments are omitted.
func TestExportedAPIMatchesBaseline(t *testing.T) {
	got := exportedAPISnapshot(t)
	path := filepath.Join(moduleRoot(t), "internal", "arch", exportedAPIBaseline)
	if *updateAPI {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create API baseline directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write API baseline: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Core API baseline: %v", err)
	}
	if got == string(want) {
		return
	}
	t.Fatalf("Core exported API changed without a reviewed baseline update:\n%s\nreview the diff, update migration/release notes, then run: go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api", apiDelta(string(want), got))
}

func exportedAPISnapshot(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var entries []string

	for _, path := range productionGoFiles(t) {
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			t.Fatalf("make %s relative to Core: %v", path, err)
		}
		packagePath := filepath.ToSlash(dir)
		if _, public := targetPublicPackages[packagePath]; !public {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s for API snapshot: %v", path, err)
		}
		for _, declaration := range file.Decls {
			for _, item := range publicDeclarations(declaration) {
				entries = append(entries, packagePath+": "+canonicalDeclaration(t, fset, item))
			}
		}
	}

	sort.Strings(entries)
	return "# Generated Core exported API baseline. Review every diff; do not edit by hand.\n" +
		"# Regenerate: go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api\n\n" +
		strings.Join(entries, "\n") + "\n"
}

func publicDeclarations(declaration ast.Decl) []ast.Node {
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		if !ast.IsExported(typed.Name.Name) {
			return nil
		}
		copy := *typed
		copy.Doc = nil
		copy.Body = nil
		return []ast.Node{&copy}
	case *ast.GenDecl:
		switch typed.Tok {
		case token.TYPE:
			var nodes []ast.Node
			for _, specification := range typed.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if !ast.IsExported(typeSpec.Name.Name) {
					continue
				}
				copy := *typeSpec
				copy.Doc = nil
				copy.Comment = nil
				nodes = append(nodes, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&copy}})
			}
			return nodes
		case token.CONST, token.VAR:
			if !generalDeclarationExportsName(typed) {
				return nil
			}
			copy := &ast.GenDecl{Tok: typed.Tok, Lparen: typed.Lparen}
			for _, specification := range typed.Specs {
				valueSpec := specification.(*ast.ValueSpec)
				valueCopy := *valueSpec
				valueCopy.Doc = nil
				valueCopy.Comment = nil
				copy.Specs = append(copy.Specs, &valueCopy)
			}
			return []ast.Node{copy}
		}
	}
	return nil
}

func generalDeclarationExportsName(declaration *ast.GenDecl) bool {
	for _, specification := range declaration.Specs {
		for _, name := range specification.(*ast.ValueSpec).Names {
			if ast.IsExported(name.Name) {
				return true
			}
		}
	}
	return false
}

func canonicalDeclaration(t *testing.T, fset *token.FileSet, declaration ast.Node) string {
	t.Helper()
	var output bytes.Buffer
	if err := format.Node(&output, fset, declaration); err != nil {
		t.Fatalf("format API declaration: %v", err)
	}
	return strings.Join(strings.Fields(output.String()), " ")
}

func apiDelta(want, got string) string {
	wantLines := lineCounts(want)
	gotLines := lineCounts(got)
	var removed, added []string
	for line, count := range wantLines {
		for range max(0, count-gotLines[line]) {
			removed = append(removed, "- "+line)
		}
	}
	for line, count := range gotLines {
		for range max(0, count-wantLines[line]) {
			added = append(added, "+ "+line)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	changes := append(removed, added...)
	const limit = 80
	if len(changes) > limit {
		changes = append(changes[:limit], fmt.Sprintf("... %d additional changed lines", len(changes)-limit))
	}
	return strings.Join(changes, "\n")
}

func lineCounts(value string) map[string]int {
	counts := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		counts[line]++
	}
	return counts
}
