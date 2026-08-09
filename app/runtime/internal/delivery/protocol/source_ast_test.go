package protocol

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func parseProtocolSource(t *testing.T) []*ast.File {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package files: %v", err)
	}
	slices.Sort(files)

	parsed := make([]*ast.File, 0, len(files))
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		parsed = append(parsed, syntax)
	}
	return parsed
}

func constantSpecs(syntax *ast.File) []*ast.ValueSpec {
	var constants []*ast.ValueSpec
	for _, declaration := range syntax.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, specification := range group.Specs {
			if value, ok := specification.(*ast.ValueSpec); ok {
				constants = append(constants, value)
			}
		}
	}
	return constants
}
