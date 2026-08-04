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

// TestCrossRingAdapterConstructorsReturnConcreteImplementations keeps the
// composition root responsible for assigning implementations to
// consumer-owned ports. Returning the consumer interface from an adapter
// constructor hides its own vocabulary and reverses the dependency at the
// construction boundary.
func TestCrossRingAdapterConstructorsReturnConcreteImplementations(t *testing.T) {
	root := moduleRoot(t)
	tests := []struct {
		path, constructor, result string
	}{
		{"internal/adapter/toolset/registry.go", "NewDiagnosticRegistry", "DiagnosticRegistry"},
		{"internal/adapter/workspace/session_checkpoints.go", "NewSessionCheckpoints", "SessionCheckpoints"},
		{"internal/adapter/agentexec/turn/session_cleanup.go", "NewSessionTurnCleanup", "SessionTurnCleanup"},
		{"internal/bootstrap/hooks.go", "NewHookResolver", "*adapterhooks.Resolver"},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, test.path), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", test.path, err)
		}
		found := false
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Name.Name != test.constructor {
				continue
			}
			found = true
			if function.Type.Results == nil || len(function.Type.Results.List) != 1 {
				t.Errorf("%s: %s must return exactly one concrete implementation", test.path, test.constructor)
				break
			}
			if got := exprString(function.Type.Results.List[0].Type); got != test.result {
				t.Errorf("%s: %s returns %s, want concrete %s", test.path, test.constructor, got, test.result)
			}
		}
		if !found {
			t.Errorf("%s: constructor %s not found", test.path, test.constructor)
		}
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
