package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAmbientContextRootsBelongToProcessOwners keeps request and component
// code from silently inventing an immortal lifetime. The allowlist names
// lifecycle behaviors rather than files, so declarations may move without
// changing the rule: Runtime Instance/Host and the process HTTP transport are
// the only owners that may start from an ambient root.
func TestAmbientContextRootsBelongToProcessOwners(t *testing.T) {
	root := moduleRoot(t)
	allowed := map[string]string{
		"internal/bootstrap:OpenInstance":            "Runtime instance lifetime",
		"internal/bootstrap:closeHostLifetime":       "Host shutdown budget",
		"internal/bootstrap:Instance.Close":          "Instance shutdown budget",
		"internal/delivery/transport/http:NewServer": "process HTTP transport lifetime",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		contextAliases := importedAliases(file, "context")
		if len(contextAliases) == 0 {
			return nil
		}
		directory, relativeErr := filepath.Rel(root, filepath.Dir(path))
		if relativeErr != nil {
			return relativeErr
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			owner := filepath.ToSlash(directory) + ":" + function.Name.Name
			if receiver := receiverName(function.Recv); receiver != "" {
				owner = filepath.ToSlash(directory) + ":" + receiver + "." + function.Name.Name
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (selector.Sel.Name != "Background" && selector.Sel.Name != "TODO") {
					return true
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				if _, isContext := contextAliases[qualifier.Name]; !isContext {
					return true
				}
				if reason := allowed[owner]; reason == "" {
					relative, _ := filepath.Rel(root, path)
					t.Errorf("%s: %s creates context.%s outside a process lifecycle owner", relative, owner, selector.Sel.Name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Runtime context roots: %v", err)
	}
}

// TestExecutableOrchestrationComplexityBudget applies a uniform control-flow
// budget to production orchestration. It deliberately excludes generated
// sources, declaration catalogs, and recursive validators; moving a function
// to another file or package does not reset the budget.
func TestExecutableOrchestrationComplexityBudget(t *testing.T) {
	root := moduleRoot(t)
	const maxCyclomaticComplexity = 32
	for _, ring := range []string{"application", "adapter", "delivery", "bootstrap"} {
		directory := filepath.Join(root, "internal", ring)
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if generatedGoSource(source) {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, source, 0)
			if parseErr != nil {
				return parseErr
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil || declarationCatalog(function) || recursiveValidator(function) {
					continue
				}
				complexity := cyclomaticComplexity(function.Body)
				if complexity > maxCyclomaticComplexity {
					relative, _ := filepath.Rel(root, path)
					t.Errorf(
						"%s: %s has control-flow complexity %d, budget %d",
						relative,
						function.Name.Name,
						complexity,
						maxCyclomaticComplexity,
					)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s orchestration complexity: %v", ring, err)
		}
	}
}

func importedAliases(file *ast.File, importPath string) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		alias := filepath.Base(importPath)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		aliases[alias] = struct{}{}
	}
	return aliases
}

func generatedGoSource(source []byte) bool {
	const marker = "Code generated "
	limit := min(len(source), 2048)
	return strings.Contains(string(source[:limit]), marker) &&
		strings.Contains(string(source[:limit]), " DO NOT EDIT.")
}

func declarationCatalog(function *ast.FuncDecl) bool {
	return strings.HasPrefix(function.Name.Name, "register")
}

func recursiveValidator(function *ast.FuncDecl) bool {
	if !strings.Contains(strings.ToLower(function.Name.Name), "validat") {
		return false
	}
	recursive := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := call.Fun.(*ast.Ident)
		if ok && callee.Name == function.Name.Name {
			recursive = true
		}
		return !recursive
	})
	return recursive
}

func cyclomaticComplexity(body *ast.BlockStmt) int {
	complexity := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		case *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if value.Op == token.LAND || value.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}
