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

// TestInnerQueriesDoNotUseJavaGetterNames keeps internal semantic APIs on Go
// vocabulary. Delivery is intentionally excluded because its exported method
// names mirror the published RPC operations; inner queries instead use a noun,
// Lookup, or Find according to their missing-value contract.
func TestInnerQueriesDoNotUseJavaGetterNames(t *testing.T) {
	root := moduleRoot(t)
	for _, ring := range []string{"domain", "application", "adapter", "infra"} {
		t.Run(ring, func(t *testing.T) {
			dir := filepath.Join(root, "internal", ring)
			err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
				if err != nil {
					return err
				}
				check := func(name string) {
					if strings.HasPrefix(name, "Get") && len(name) > len("Get") && ast.IsExported(name[len("Get"):]) {
						t.Errorf("%s declares Java-style query %s; name the value or its lookup semantics", path, name)
					}
				}
				for _, declaration := range file.Decls {
					switch typed := declaration.(type) {
					case *ast.FuncDecl:
						check(typed.Name.Name)
					case *ast.GenDecl:
						for _, spec := range typed.Specs {
							typeSpec, ok := spec.(*ast.TypeSpec)
							if !ok {
								continue
							}
							contract, ok := typeSpec.Type.(*ast.InterfaceType)
							if !ok {
								continue
							}
							for _, method := range contract.Methods.List {
								for _, name := range method.Names {
									check(name.Name)
								}
							}
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s naming: %v", ring, err)
			}
		})
	}
}

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

// TestExportedCatalogsAreNotMutableGlobals keeps closed vocabularies and
// default construction values from becoming process-wide writable state. A
// caller may mutate its own snapshot, never the fact another boundary reads.
func TestExportedCatalogsAreNotMutableGlobals(t *testing.T) {
	root := moduleRoot(t)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				values, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range values.Names {
					if !ast.IsExported(name.Name) {
						continue
					}
					mutable := isMutableCatalogType(values.Type)
					if index < len(values.Values) {
						mutable = mutable || isMutableCatalogValue(values.Values[index])
					} else if len(values.Values) == 1 {
						mutable = mutable || isMutableCatalogValue(values.Values[0])
					}
					if mutable {
						relative, _ := filepath.Rel(root, path)
						t.Errorf("%s: exported var %s is mutable package state; return a caller-owned value instead", relative, name.Name)
					}
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk runtime: %v", walkErr)
	}
}

func isMutableCatalogType(expression ast.Expr) bool {
	switch expression.(type) {
	case *ast.ArrayType, *ast.MapType, *ast.StructType:
		return true
	default:
		return false
	}
}

func isMutableCatalogValue(expression ast.Expr) bool {
	_, mutable := expression.(*ast.CompositeLit)
	return mutable
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
