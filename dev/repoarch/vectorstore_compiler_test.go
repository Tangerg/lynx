package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestVectorStoreProviderCompilersStayPrivate(t *testing.T) {
	t.Parallel()
	root := filepath.Join(repositoryRoot(t), "vectorstores")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "visitor") ||
			filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if ok && typeSpec.Name.Name == "Visitor" {
						t.Errorf("%s:%d exports provider compiler type Visitor", filepath.ToSlash(path), fset.Position(typeSpec.Pos()).Line)
					}
				}
			case *ast.FuncDecl:
				if value.Recv == nil && value.Name.Name == "NewVisitor" {
					t.Errorf("%s:%d exports provider compiler constructor NewVisitor", filepath.ToSlash(path), fset.Position(value.Pos()).Line)
				}
				if receiverNamed(value.Recv, "visitor") && value.Name.IsExported() && value.Name.Name != "Visit" {
					t.Errorf("%s:%d exports provider compiler method %s", filepath.ToSlash(path), fset.Position(value.Pos()).Line, value.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func receiverNamed(receiver *ast.FieldList, name string) bool {
	if receiver == nil || len(receiver.List) != 1 {
		return false
	}
	expression := receiver.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}
