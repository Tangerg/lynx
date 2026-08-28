// Package providerconformance_test locks cross-provider constructor contracts.
package providerconformance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProviderConstructorsAreSelfCovering(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve provider conformance source path")
	}
	modelsRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "models"))
	providers, err := filepath.Glob(filepath.Join(modelsRoot, "*"))
	if err != nil {
		t.Fatal(err)
	}

	constructors := 0
	for _, provider := range providers {
		info, err := os.Stat(provider)
		if err != nil || !info.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(provider, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		validateReceivers := map[string]struct{}{}
		var declarations []*ast.FuncDecl
		fileSet := token.NewFileSet()
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(fileSet, file, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok {
					continue
				}
				declarations = append(declarations, function)
				if function.Name.Name == "Validate" && function.Recv != nil && len(function.Recv.List) == 1 {
					if receiver := namedType(function.Recv.List[0].Type); receiver != "" {
						validateReceivers[receiver] = struct{}{}
					}
				}
			}
		}

		for _, function := range declarations {
			if function.Recv != nil || !strings.HasPrefix(function.Name.Name, "New") || !function.Name.IsExported() {
				continue
			}
			configType, parameterCount := constructorConfig(function.Type.Params)
			if configType == "" {
				continue
			}
			constructors++
			if parameterCount != 1 && parameterCount != 2 {
				t.Errorf("%s: constructor with config has %d parameters", function.Name.Name, parameterCount)
			}
			if parameterCount == 2 && !startsWithContext(function.Type.Params) {
				t.Errorf("%s: only context.Context may precede config", function.Name.Name)
			}
			if _, ok := validateReceivers[configType]; !ok {
				t.Errorf("%s: %s does not own Validate", function.Name.Name, configType)
			}
			if !returnsValueAndError(function.Type.Results) {
				t.Errorf("%s: constructor must return value and error", function.Name.Name)
			}
		}
	}
	if constructors < 70 {
		t.Fatalf("discovered %d provider constructors, want at least 70", constructors)
	}
}

func constructorConfig(parameters *ast.FieldList) (string, int) {
	if parameters == nil {
		return "", 0
	}
	count := 0
	configType := ""
	for _, field := range parameters.List {
		fieldCount := max(1, len(field.Names))
		count += fieldCount
		if name := namedType(field.Type); strings.HasSuffix(name, "Config") {
			configType = name
		}
	}
	return configType, count
}

func namedType(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return namedType(value.X)
	default:
		return ""
	}
}

func startsWithContext(parameters *ast.FieldList) bool {
	if parameters == nil || len(parameters.List) == 0 {
		return false
	}
	selector, ok := parameters.List[0].Type.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "context"
}

func returnsValueAndError(results *ast.FieldList) bool {
	if results == nil || len(results.List) != 2 {
		return false
	}
	errorType, ok := results.List[1].Type.(*ast.Ident)
	return ok && errorType.Name == "error"
}
