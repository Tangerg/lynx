package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

const modulePrefix = "github.com/Tangerg/lynx/"

type repositoryModule struct {
	path string
	dir  string
	file *modfile.File
	tier int
}

func TestWorkspaceCoversEveryProductModule(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverProductModules(t, root)

	data, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		t.Fatal(err)
	}

	workspaceDirs := make(map[string]struct{}, len(work.Use))
	for _, use := range work.Use {
		dir := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(use.Path, "./")))
		if _, duplicate := workspaceDirs[dir]; duplicate {
			t.Errorf("go.work lists %q more than once", dir)
		}
		workspaceDirs[dir] = struct{}{}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), "go.mod")); err != nil {
			t.Errorf("go.work entry %q has no readable go.mod: %v", dir, err)
		}
	}

	for _, module := range modules {
		if _, ok := workspaceDirs[module.dir]; !ok {
			t.Errorf("product module %s (%s) is missing from go.work", module.path, module.dir)
		}
		if module.file.Go == nil || work.Go == nil || module.file.Go.Version != work.Go.Version {
			var moduleVersion string
			if module.file.Go != nil {
				moduleVersion = module.file.Go.Version
			}
			var workspaceVersion string
			if work.Go != nil {
				workspaceVersion = work.Go.Version
			}
			t.Errorf("%s go version = %q; workspace = %q", module.path, moduleVersion, workspaceVersion)
		}
	}
}

func TestProductModuleGraphIsLayeredAndAcyclic(t *testing.T) {
	t.Parallel()
	modules := discoverProductModules(t, repositoryRoot(t))
	graph := make(map[string][]string, len(modules))

	for _, module := range modules {
		if len(module.file.Replace) != 0 {
			t.Errorf("%s uses replace; release modules must resolve without workspace-only paths", module.path)
		}
		for _, requirement := range module.file.Require {
			dependency := requirement.Mod.Path
			if !strings.HasPrefix(dependency, modulePrefix) {
				continue
			}
			dependencyModule, ok := modules[dependency]
			if !ok {
				t.Errorf("%s requires undiscovered Lynx module %s", module.path, dependency)
				continue
			}
			graph[module.path] = append(graph[module.path], dependency)
			if module.tier <= dependencyModule.tier {
				t.Errorf("reverse or peer dependency: tier %d %s -> tier %d %s",
					module.tier, module.path, dependencyModule.tier, dependency)
			}
		}
	}

	assertAcyclic(t, graph, modules)
}

func TestFoundationDependencyBudgets(t *testing.T) {
	t.Parallel()
	modules := discoverProductModules(t, repositoryRoot(t))
	budgets := map[string][]string{
		modulePrefix + "core":                     {},
		modulePrefix + "tokenizer":                {},
		modulePrefix + "tool":                     {modulePrefix + "core"},
		modulePrefix + "chatclient":               {modulePrefix + "core"},
		modulePrefix + "embeddingclient":          {modulePrefix + "core"},
		modulePrefix + "documentreaders":          {modulePrefix + "core"},
		modulePrefix + "internal/chathistorykit":  {modulePrefix + "core"},
		modulePrefix + "internal/chathistoryotel": {"go.opentelemetry.io/otel", "go.opentelemetry.io/otel/trace"},
		modulePrefix + "internal/vectorstorekit":  {modulePrefix + "core"},
		modulePrefix + "chathistory":              {modulePrefix + "core", modulePrefix + "internal/chathistorykit"},
	}

	for modulePath, want := range budgets {
		module, ok := modules[modulePath]
		if !ok {
			t.Errorf("dependency budget names missing module %s", modulePath)
			continue
		}
		var got []string
		for _, requirement := range module.file.Require {
			if !requirement.Indirect {
				got = append(got, requirement.Mod.Path)
			}
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s direct dependency budget changed\nwant: %v\n got: %v", modulePath, want, got)
		}
	}
}

func TestIntegrationFamiliesHaveIndependentBoundaries(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, family := range []string{"vectorstores", "chathistorystores"} {
		entries, err := os.ReadDir(filepath.Join(root, family))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dir := filepath.Join(root, family, entry.Name())
			matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) == 0 {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
				t.Errorf("%s/%s contains Go code without an independent go.mod", family, entry.Name())
				continue
			}
			if family == "vectorstores" {
				assertVectorConformance(t, dir)
			}
			assertLeafSourceBoundary(t, family, entry.Name(), dir)
		}
	}
}

func assertVectorConformance(t *testing.T, dir string) {
	t.Helper()
	filename := filepath.Join(dir, "conformance_test.go")
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Errorf("%s: missing or invalid conformance suite: %v", filepath.Base(dir), err)
		return
	}

	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != modulePrefix+"internal/vectorstorekit/conformance" {
			continue
		}
		alias := "conformance"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = struct{}{}
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Run" {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok {
			_, found = aliases[identifier.Name]
		}
		return !found
	})
	if !found {
		t.Errorf("%s must call vectorstorekit/conformance.Run", filepath.Base(dir))
	}
}

func assertLeafSourceBoundary(t *testing.T, family, leaf, dir string) {
	t.Helper()
	modulePath := modulePrefix + family + "/" + leaf
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		imports := make(map[string]string, len(file.Imports))
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(importPath, modulePrefix+family+"/") &&
				importPath != modulePath && !strings.HasPrefix(importPath, modulePath+"/") {
				t.Errorf("%s imports sibling integration %s", filepath.ToSlash(path), importPath)
			}
			if family == "vectorstores" && strings.HasPrefix(importPath, "go.opentelemetry.io/otel") {
				t.Errorf("%s imports OpenTelemetry; decorate stores in the otel module", filepath.ToSlash(path))
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}

		if strings.HasSuffix(path, "_test.go") || family != "vectorstores" {
			return nil
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.TypeSpec:
				if node.Name.Name == "StoreConfig" {
					checkStoreConfig(t, path, imports, node)
				}
			case *ast.CallExpr:
				selector, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					break
				}
				identifier, ok := selector.X.(*ast.Ident)
				if ok && imports[identifier.Name] == "github.com/google/uuid" &&
					(selector.Sel.Name == "New" || selector.Sel.Name == "NewString") {
					t.Errorf("%s generates document IDs; stores must preserve caller identity", filepath.ToSlash(path))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Errorf("walk %s: %v", dir, err)
	}
}

func checkStoreConfig(t *testing.T, path string, imports map[string]string, spec *ast.TypeSpec) {
	t.Helper()
	structure, ok := spec.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range structure.Fields.List {
		for _, name := range field.Names {
			if name.Name == "StoreDocumentContent" {
				t.Errorf("%s exposes lossy StoreDocumentContent", filepath.ToSlash(path))
			}
		}
		selector, ok := field.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Context" {
			continue
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && imports[identifier.Name] == "context" {
			t.Errorf("%s retains context.Context in StoreConfig", filepath.ToSlash(path))
		}
	}
}

func discoverProductModules(t *testing.T, root string) map[string]repositoryModule {
	t.Helper()
	modules := make(map[string]repositoryModule)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			first := strings.Split(filepath.ToSlash(relative), "/")[0]
			if first == ".git" || first == "app" || entry.Name() == "vendor" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := modfile.Parse(path, data, nil)
		if err != nil {
			return err
		}
		if parsed.Module == nil {
			t.Errorf("%s has no module directive", path)
			return nil
		}
		relativeDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		relativeDir = filepath.ToSlash(relativeDir)
		modulePath := parsed.Module.Mod.Path
		if want := modulePrefix + relativeDir; modulePath != want {
			t.Errorf("module path does not match repository directory: %s; want %s", modulePath, want)
		}
		tier := moduleTier(modulePath)
		if tier < 0 {
			t.Errorf("module %s has no declared architecture tier", modulePath)
		}
		if previous, exists := modules[modulePath]; exists {
			t.Errorf("duplicate module path %s in %s and %s", modulePath, previous.dir, relativeDir)
		}
		modules[modulePath] = repositoryModule{path: modulePath, dir: relativeDir, file: parsed, tier: tier}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return modules
}

func moduleTier(path string) int {
	switch path {
	case modulePrefix + "core", modulePrefix + "tokenizer", modulePrefix + "skills", modulePrefix + "internal/chathistoryotel":
		return 0
	case modulePrefix + "tool", modulePrefix + "chatclient", modulePrefix + "embeddingclient",
		modulePrefix + "documentreaders", modulePrefix + "internal/chathistorykit",
		modulePrefix + "internal/vectorstorekit", modulePrefix + "models/protocol/openai":
		return 1
	case modulePrefix + "a2a", modulePrefix + "agent", modulePrefix + "chathistory",
		modulePrefix + "documentpipeline", modulePrefix + "internal/vectorstorepg",
		modulePrefix + "mcp", modulePrefix + "models", modulePrefix + "otel",
		modulePrefix + "rag", modulePrefix + "tools":
		return 2
	case modulePrefix + "internal/repoarch", modulePrefix + "examples/mcp":
		return 4
	}
	for _, prefix := range []string{
		modulePrefix + "chathistorystores/",
		modulePrefix + "documentpipeline/",
		modulePrefix + "documentreaders/",
		modulePrefix + "models/",
		modulePrefix + "tokenizer/",
		modulePrefix + "vectorstores/",
	} {
		if strings.HasPrefix(path, prefix) {
			return 3
		}
	}
	return -1
}

func assertAcyclic(t *testing.T, graph map[string][]string, modules map[string]repositoryModule) {
	t.Helper()
	state := make(map[string]uint8, len(modules))
	var stack []string
	var visit func(string)
	visit = func(module string) {
		switch state[module] {
		case 1:
			start := slices.Index(stack, module)
			cycle := append(slices.Clone(stack[start:]), module)
			t.Errorf("module dependency cycle: %s", strings.Join(cycle, " -> "))
			return
		case 2:
			return
		}
		state[module] = 1
		stack = append(stack, module)
		for _, dependency := range graph[module] {
			visit(dependency)
		}
		stack = stack[:len(stack)-1]
		state[module] = 2
	}
	for module := range modules {
		visit(module)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
