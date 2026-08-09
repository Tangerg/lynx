package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const oldAgentModulePath = "github.com/Tangerg/lynx/agent"

func TestOldAgentModuleIsAbsent(t *testing.T) {
	root := moduleRoot(t)
	err := walkGoFiles(root, func(path string, file *ast.File) {
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath != oldAgentModulePath && !strings.HasPrefix(importPath, oldAgentModulePath+"/") {
				continue
			}
			relativePath, _ := filepath.Rel(root, path)
			t.Errorf("old Agent import remains after P8 cutover: %s", filepath.ToSlash(relativePath))
		}
	})
	if err != nil {
		t.Fatalf("scan old Agent imports: %v", err)
	}
}

type temporaryDomainPort struct {
	owner       string
	deletePhase string
}

var temporaryDomainIOPorts = map[string]temporaryDomainPort{}

func TestTemporaryDomainIOPortsAreExact(t *testing.T) {
	root := moduleRoot(t)
	domainRoot := filepath.Join(root, "internal", "domain")
	for name, port := range temporaryDomainIOPorts {
		if port.owner == "" || port.deletePhase == "" {
			t.Errorf("Domain I/O port %s lacks an owner or deletion phase", name)
		}
	}
	actual := make(map[string]struct{})
	err := walkGoFiles(domainRoot, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				named, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				contract, ok := named.Type.(*ast.InterfaceType)
				if !ok || !interfaceUsesContext(contract) {
					continue
				}
				relativePath, _ := filepath.Rel(root, path)
				actual[filepath.ToSlash(relativePath)+":"+named.Name.Name] = struct{}{}
			}
		}
	})
	if err != nil {
		t.Fatalf("scan Domain I/O ports: %v", err)
	}

	compareTemporarySet(t, "Domain I/O port", actual, temporaryDomainIOPorts)
}

func interfaceUsesContext(contract *ast.InterfaceType) bool {
	if contract.Methods == nil {
		return false
	}
	for _, method := range contract.Methods.List {
		usesContext := false
		ast.Inspect(method.Type, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Context" {
				return true
			}
			packageName, ok := selector.X.(*ast.Ident)
			if ok && packageName.Name == "context" {
				usesContext = true
			}
			return true
		})
		if usesContext {
			return true
		}
	}
	return false
}

type temporaryComponent struct {
	owner       string
	deletePhase string
}

var temporaryComponents = map[string]temporaryComponent{
	"completion":   {"internal/component/completion", "P9"},
	"httporigin":   {"internal/component/httporigin", "P9"},
	"idempotency":  {"internal/component/idempotency", "P9"},
	"keyset":       {"internal/component/keyset", "P9"},
	"pathidentity": {"internal/component/pathidentity", "P9"},
	"replaycursor": {"internal/component/replaycursor", "P9"},
	"secretmask":   {"internal/component/secretmask", "P9"},
	"shutdown":     {"internal/component/shutdown", "P9"},
	"signal":       {"internal/component/signal", "P9"},
	"taskgroup":    {"internal/component/taskgroup", "P9"},
}

func TestTemporaryComponentPackagesAreExact(t *testing.T) {
	root := moduleRoot(t)
	componentRoot := filepath.Join(root, "internal", "component")
	for name, component := range temporaryComponents {
		if component.owner == "" || component.deletePhase == "" {
			t.Errorf("component package %s lacks an owner or deletion phase", name)
		}
	}
	entries, err := os.ReadDir(componentRoot)
	if err != nil {
		t.Fatalf("read component umbrella: %v", err)
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			actual[entry.Name()] = struct{}{}
		}
	}
	compareTemporarySet(t, "component package", actual, temporaryComponents)
}

func walkGoFiles(root string, visit func(path string, file *ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	})
}

func compareTemporarySet[T any](
	t *testing.T,
	name string,
	actual map[string]struct{},
	ledger map[string]T,
) {
	t.Helper()
	for item := range actual {
		if _, allowed := ledger[item]; !allowed {
			t.Errorf("%s has no temporary owner and deletion phase: %s", name, item)
		}
	}
	for item := range ledger {
		if _, exists := actual[item]; !exists {
			t.Errorf("stale %s exception: %s", name, item)
		}
	}
}
