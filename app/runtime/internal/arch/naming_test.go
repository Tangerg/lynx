package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestReceiverNamesStayConsistent keeps a receiver's name stable across every
// production method on the same type. Mixed names make one type read like
// several unrelated abstractions and commonly expose a partial rename.
func TestReceiverNamesStayConsistent(t *testing.T) {
	root := moduleRoot(t)
	namesByType := make(map[string]map[string][]string)

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || (strings.HasPrefix(entry.Name(), ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Recv == nil || len(method.Recv.List) != 1 {
				continue
			}
			receiver := method.Recv.List[0]
			if len(receiver.Names) != 1 || receiver.Names[0].Name == "_" {
				continue
			}
			typeName := receiverTypeName(receiver.Type)
			if typeName == "" {
				continue
			}

			relativeDirectory, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			typeKey := filepath.ToSlash(filepath.Join(relativeDirectory, typeName))
			receiverName := receiver.Names[0].Name
			if namesByType[typeKey] == nil {
				namesByType[typeKey] = make(map[string][]string)
			}
			relativeFile, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			namesByType[typeKey][receiverName] = append(
				namesByType[typeKey][receiverName],
				filepath.ToSlash(relativeFile)+":"+method.Name.Name,
			)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module: %v", walkErr)
	}

	for typeName, receiverNames := range namesByType {
		if len(receiverNames) < 2 {
			continue
		}
		names := make([]string, 0, len(receiverNames))
		for name := range receiverNames {
			names = append(names, name)
		}
		slices.Sort(names)

		var locations []string
		for _, name := range names {
			slices.Sort(receiverNames[name])
			locations = append(locations, name+" in "+strings.Join(receiverNames[name], ", "))
		}
		t.Errorf(
			"%s uses inconsistent receiver names %s; choose one name that represents the type's single responsibility",
			typeName,
			strings.Join(locations, "; "),
		)
	}
}

func receiverTypeName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexListExpr:
		return receiverTypeName(typed.X)
	case *ast.ParenExpr:
		return receiverTypeName(typed.X)
	default:
		return ""
	}
}
