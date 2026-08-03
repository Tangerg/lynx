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

const repositoryModulePath = "github.com/Tangerg/lynx"

type repositoryModule struct {
	path  string
	dir   string
	file  *modfile.File
	layer int
}

type providerPackage struct {
	family     string
	relative   string
	dir        string
	importPath string
}

func TestWorkspaceCoversEveryRepositoryModule(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)

	data, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(work.Replace) != 0 {
		t.Error("go.work must not contain replace directives")
	}

	workspaceDirs := make(map[string]struct{}, len(work.Use))
	for _, use := range work.Use {
		dir := cleanWorkspacePath(use.Path)
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
			t.Errorf("module %s (%s) is missing from go.work", module.path, module.dir)
		}
		if module.file.Go == nil || work.Go == nil || module.file.Go.Version != work.Go.Version {
			moduleVersion := ""
			if module.file.Go != nil {
				moduleVersion = module.file.Go.Version
			}
			workspaceVersion := ""
			if work.Go != nil {
				workspaceVersion = work.Go.Version
			}
			t.Errorf("%s go version = %q; workspace = %q", module.path, moduleVersion, workspaceVersion)
		}
	}

	for dir := range workspaceDirs {
		if dir == "app" || strings.HasPrefix(dir, "app/") {
			continue
		}
		if _, ok := moduleByDir(modules, dir); !ok {
			t.Errorf("go.work contains non-app module %q that repository discovery did not classify", dir)
		}
	}
}

func TestModulesStayOutOfInternalDirectories(t *testing.T) {
	t.Parallel()
	for _, module := range discoverModules(t, repositoryRoot(t)) {
		if containsPathSegment(module.dir, "internal") {
			t.Errorf("module %s lives under internal; internal packages may hide implementation, but must not form dependency islands", module.path)
		}
	}
}

func TestModuleGraphHasOneWayDependencyBudgets(t *testing.T) {
	t.Parallel()
	modules := discoverModules(t, repositoryRoot(t))
	graph := make(map[string][]string, len(modules))

	for _, module := range modules {
		if len(module.file.Replace) != 0 {
			t.Errorf("%s uses replace; every module must resolve outside go.work", module.path)
		}
		allowed := allowedRepositoryDependencies(module.path)
		for _, requirement := range module.file.Require {
			dependency := requirement.Mod.Path
			if !isRepositoryImport(dependency) {
				continue
			}
			dependencyModule, ok := modules[dependency]
			if !ok {
				t.Errorf("%s requires undiscovered Lynx module %s", module.path, dependency)
				continue
			}
			graph[module.path] = append(graph[module.path], dependency)
			if _, ok := allowed[dependency]; !ok {
				t.Errorf("%s must not depend on %s; allowed Lynx modules: %v",
					module.path, dependency, mapKeys(allowed))
			}
			if module.layer <= dependencyModule.layer {
				t.Errorf("reverse or peer module dependency: layer %d %s -> layer %d %s",
					module.layer, module.path, dependencyModule.layer, dependency)
			}
		}
	}

	assertAcyclic(t, graph, modules)
}

func TestRootModuleIsStdlibOnlyAndLayered(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)
	foundation := modules[repositoryModulePath]
	if len(foundation.file.Require) != 0 {
		t.Errorf("root foundation module must not carry third-party requirements")
	}

	walkRootProductionFiles(t, root, modules, func(path, relative string, file *ast.File) {
		sourceFamily := firstPathSegment(relative)
		sourceLayer, ok := rootFamilyLayer(sourceFamily)
		if !ok {
			t.Errorf("%s belongs to an unclassified root package family %q", filepath.ToSlash(path), sourceFamily)
			return
		}

		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if !isRepositoryImport(importPath) {
				if isThirdPartyImport(importPath) {
					t.Errorf("%s imports third-party package %s; move the dependency to a nested module", filepath.ToSlash(path), importPath)
				}
				continue
			}

			owner, ok := moduleForImport(modules, importPath)
			if !ok {
				t.Errorf("%s imports unowned Lynx package %s", filepath.ToSlash(path), importPath)
				continue
			}
			if owner.path != repositoryModulePath {
				t.Errorf("%s reverses the module direction by importing higher module %s", filepath.ToSlash(path), owner.path)
				continue
			}

			dependencyFamily := importFamily(importPath)
			dependencyLayer, ok := rootFamilyLayer(dependencyFamily)
			if !ok {
				t.Errorf("%s imports unclassified root package family %q", filepath.ToSlash(path), dependencyFamily)
				continue
			}
			if sourceLayer < dependencyLayer {
				t.Errorf("reverse root package dependency: layer %d %s -> layer %d %s",
					sourceLayer, sourceFamily, dependencyLayer, dependencyFamily)
			}
			if sourceLayer == dependencyLayer && sourceFamily != dependencyFamily {
				t.Errorf("peer root package families must not couple: %s -> %s in %s",
					sourceFamily, dependencyFamily, filepath.ToSlash(path))
			}
		}
	})
}

func TestProviderPackagesAreIndependentAndConformant(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)

	for _, provider := range discoverVectorProviders(t, root) {
		assertProviderBoundary(t, provider)
		assertVectorConformance(t, provider)
		assertDependencyIsland(t, provider, modules)
	}
	for _, provider := range discoverHistoryProviders(t, root) {
		assertProviderBoundary(t, provider)
		assertDependencyIsland(t, provider, modules)
	}

	postgresRoot := filepath.Join(root, "vectorstores", "postgres")
	entries, err := os.ReadDir(postgresRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			t.Errorf("vectorstores/postgres must expose provider packages, not an aggregate root package: %s", entry.Name())
		}
	}
}

func TestIntegrationFamiliesDoNotImportSiblingProviders(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	assertFamilySiblingBoundary(t, root, "models", map[string]struct{}{
		"catalog": {}, "internal": {}, "protocol": {},
	})
	assertFamilySiblingBoundary(t, root, "tools/webfetch", map[string]struct{}{"internal": {}})
	assertFamilySiblingBoundary(t, root, "tools/websearch", map[string]struct{}{"internal": {}})
	assertFamilySiblingBoundary(t, root, "documentreaders", nil)
}

func TestRetiredLayoutsCannotReturn(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relative := range []string{
		"chatclient/middleware",
		"chathistorystores",
		"documentpipeline/id",
		"internal/chathistorykit",
		"internal/chathistoryotel",
		"internal/repoarch",
		"internal/vectorstorekit",
		"internal/vectorstorepg",
		"vectorstores/cockroachdb",
		"vectorstores/pgvector",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			t.Errorf("retired layout %s has returned", relative)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect retired layout %s: %v", relative, err)
		}
	}
}

func discoverModules(t *testing.T, root string) map[string]repositoryModule {
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
			if shouldSkipRepositoryDir(filepath.ToSlash(relative), entry.Name()) {
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
		wantPath := repositoryModulePath
		if relativeDir != "." {
			wantPath += "/" + relativeDir
		}
		modulePath := parsed.Module.Mod.Path
		if modulePath != wantPath {
			t.Errorf("module path does not match directory: %s; want %s", modulePath, wantPath)
		}
		layer := moduleLayer(modulePath)
		if layer < 0 {
			t.Errorf("module %s has no declared architecture layer", modulePath)
		}
		if previous, exists := modules[modulePath]; exists {
			t.Errorf("duplicate module path %s in %s and %s", modulePath, previous.dir, relativeDir)
		}
		modules[modulePath] = repositoryModule{path: modulePath, dir: relativeDir, file: parsed, layer: layer}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return modules
}

func moduleLayer(path string) int {
	switch path {
	case repositoryModulePath, repositoryModulePath + "/skills":
		return 0
	case repositoryModulePath + "/documentpipeline/markdown",
		repositoryModulePath + "/models",
		repositoryModulePath + "/models/google",
		repositoryModulePath + "/models/ollama",
		repositoryModulePath + "/tools":
		return 2
	case repositoryModulePath + "/examples/mcp":
		return 3
	case repositoryModulePath + "/dev/repoarch":
		return 4
	default:
		if strings.HasPrefix(path, repositoryModulePath+"/") {
			return 1
		}
		return -1
	}
}

func allowedRepositoryDependencies(path string) map[string]struct{} {
	dependencies := func(paths ...string) map[string]struct{} {
		result := make(map[string]struct{}, len(paths))
		for _, dependency := range paths {
			result[dependency] = struct{}{}
		}
		return result
	}
	switch path {
	case repositoryModulePath,
		repositoryModulePath + "/skills",
		repositoryModulePath + "/dev/repoarch":
		return dependencies()
	case repositoryModulePath + "/documentpipeline/markdown":
		return dependencies(repositoryModulePath, repositoryModulePath+"/documentpipeline")
	case repositoryModulePath + "/models",
		repositoryModulePath + "/models/google",
		repositoryModulePath + "/models/ollama":
		return dependencies(repositoryModulePath, repositoryModulePath+"/models/protocol/openai")
	case repositoryModulePath + "/tools":
		return dependencies(repositoryModulePath, repositoryModulePath+"/skills")
	case repositoryModulePath + "/examples/mcp":
		return dependencies(
			repositoryModulePath,
			repositoryModulePath+"/agent",
			repositoryModulePath+"/mcp",
			repositoryModulePath+"/tools",
		)
	default:
		return dependencies(repositoryModulePath)
	}
}

func rootFamilyLayer(family string) (int, bool) {
	switch family {
	case "core":
		return 0, true
	case "chatclient", "chathistory", "documentreaders", "embeddingclient", "tokenizer", "tool":
		return 1, true
	case "vectorstores":
		return 2, true
	default:
		return -1, false
	}
}

func walkRootProductionFiles(
	t *testing.T,
	root string,
	modules map[string]repositoryModule,
	visit func(path, relative string, file *ast.File),
) {
	t.Helper()
	nestedModules := make(map[string]struct{})
	for _, module := range modules {
		if module.dir != "." {
			nestedModules[module.dir] = struct{}{}
		}
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." {
				if _, nested := nestedModules[relative]; nested {
					return filepath.SkipDir
				}
			}
			if shouldSkipRepositoryDir(relative, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		visit(path, relative, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func discoverVectorProviders(t *testing.T, root string) []providerPackage {
	t.Helper()
	familyDir := filepath.Join(root, "vectorstores")
	entries, err := os.ReadDir(familyDir)
	if err != nil {
		t.Fatal(err)
	}
	var providers []providerPackage
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "storetest" || entry.Name() == "internal" {
			continue
		}
		if entry.Name() != "postgres" {
			dir := filepath.Join(familyDir, entry.Name())
			if hasProductionGoFiles(t, dir) {
				providers = append(providers, newProvider("vectorstores", entry.Name(), dir))
			}
			continue
		}

		postgresEntries, err := os.ReadDir(filepath.Join(familyDir, "postgres"))
		if err != nil {
			t.Fatal(err)
		}
		for _, postgresEntry := range postgresEntries {
			if !postgresEntry.IsDir() || postgresEntry.Name() == "internal" {
				continue
			}
			relative := "postgres/" + postgresEntry.Name()
			dir := filepath.Join(familyDir, filepath.FromSlash(relative))
			if hasProductionGoFiles(t, dir) {
				providers = append(providers, newProvider("vectorstores", relative, dir))
			}
		}
	}
	slices.SortFunc(providers, func(a, b providerPackage) int { return strings.Compare(a.relative, b.relative) })
	return providers
}

func discoverHistoryProviders(t *testing.T, root string) []providerPackage {
	t.Helper()
	familyDir := filepath.Join(root, "chathistory")
	entries, err := os.ReadDir(familyDir)
	if err != nil {
		t.Fatal(err)
	}
	var providers []providerPackage
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "internal" {
			continue
		}
		dir := filepath.Join(familyDir, entry.Name())
		if hasProductionGoFiles(t, dir) {
			providers = append(providers, newProvider("chathistory", entry.Name(), dir))
		}
	}
	slices.SortFunc(providers, func(a, b providerPackage) int { return strings.Compare(a.relative, b.relative) })
	return providers
}

func newProvider(family, relative, dir string) providerPackage {
	return providerPackage{
		family:     family,
		relative:   relative,
		dir:        dir,
		importPath: repositoryModulePath + "/" + family + "/" + relative,
	}
}

func assertProviderBoundary(t *testing.T, provider providerPackage) {
	t.Helper()
	entries, err := os.ReadDir(provider.dir)
	if err != nil {
		t.Error(err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		path := filepath.Join(provider.dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if !strings.HasSuffix(path, "_test.go") && strings.HasPrefix(importPath, "go.opentelemetry.io/otel") {
				t.Errorf("%s imports OpenTelemetry; provider observability belongs in the otel module", filepath.ToSlash(path))
			}
			assertNoProviderSiblingImport(t, provider, path, importPath)
		}
	}
}

func assertNoProviderSiblingImport(t *testing.T, provider providerPackage, path, importPath string) {
	t.Helper()
	prefix := repositoryModulePath + "/" + provider.family + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return
	}
	target := strings.TrimPrefix(importPath, prefix)
	if target == provider.relative || strings.HasPrefix(target, provider.relative+"/") {
		return
	}
	if provider.family == "vectorstores" {
		if target == "storetest" && strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.HasPrefix(provider.relative, "postgres/") && strings.HasPrefix(target, "postgres/internal/") {
			return
		}
	}
	t.Errorf("%s imports sibling provider %s", filepath.ToSlash(path), importPath)
}

func assertVectorConformance(t *testing.T, provider providerPackage) {
	t.Helper()
	path := filepath.Join(provider.dir, "conformance_test.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Errorf("%s: missing or invalid conformance suite: %v", provider.importPath, err)
		return
	}

	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != repositoryModulePath+"/vectorstores/storetest" {
			continue
		}
		alias := "storetest"
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
		t.Errorf("%s must call vectorstores/storetest.Run", provider.importPath)
	}
}

func assertDependencyIsland(t *testing.T, provider providerPackage, modules map[string]repositoryModule) {
	t.Helper()
	if !hasThirdPartyProductionImport(t, provider.dir) {
		return
	}
	relative, err := filepath.Rel(repositoryRoot(t), provider.dir)
	if err != nil {
		t.Error(err)
		return
	}
	relative = filepath.ToSlash(relative)
	owner, ok := moduleForDirectory(modules, relative)
	if !ok {
		t.Errorf("%s has third-party imports without a module owner", provider.importPath)
		return
	}
	wantDir := provider.family + "/" + provider.relative
	if provider.family == "vectorstores" && strings.HasPrefix(provider.relative, "postgres/") {
		wantDir = "vectorstores/postgres"
	}
	if owner.dir != wantDir {
		t.Errorf("%s third-party dependency island is owned by %s; want module at %s",
			provider.importPath, owner.dir, wantDir)
	}
}

func assertFamilySiblingBoundary(t *testing.T, root, family string, shared map[string]struct{}) {
	t.Helper()
	familyDir := filepath.Join(root, filepath.FromSlash(family))
	err := filepath.WalkDir(familyDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		relative, err := filepath.Rel(familyDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 {
			return nil
		}
		source := parts[0]
		if _, isShared := shared[source]; isShared {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		prefix := repositoryModulePath + "/" + family + "/"
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !strings.HasPrefix(importPath, prefix) {
				continue
			}
			target := firstPathSegment(strings.TrimPrefix(importPath, prefix))
			if target == source {
				continue
			}
			if _, isShared := shared[target]; isShared {
				continue
			}
			t.Errorf("%s imports sibling provider %s", filepath.ToSlash(path), importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func hasProductionGoFiles(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" && !strings.HasSuffix(entry.Name(), "_test.go") {
			return true
		}
	}
	return false
}

func hasThirdPartyProductionImport(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err == nil && !isRepositoryImport(importPath) && isThirdPartyImport(importPath) {
				return true
			}
		}
	}
	return false
}

func moduleForDirectory(modules map[string]repositoryModule, dir string) (repositoryModule, bool) {
	var best repositoryModule
	found := false
	for _, module := range modules {
		if module.dir == "." {
			if !found {
				best, found = module, true
			}
			continue
		}
		if dir != module.dir && !strings.HasPrefix(dir, module.dir+"/") {
			continue
		}
		if !found || len(module.dir) > len(best.dir) {
			best, found = module, true
		}
	}
	return best, found
}

func moduleForImport(modules map[string]repositoryModule, importPath string) (repositoryModule, bool) {
	var best repositoryModule
	found := false
	for path, module := range modules {
		if importPath != path && !strings.HasPrefix(importPath, path+"/") {
			continue
		}
		if !found || len(path) > len(best.path) {
			best, found = module, true
		}
	}
	return best, found
}

func moduleByDir(modules map[string]repositoryModule, dir string) (repositoryModule, bool) {
	for _, module := range modules {
		if module.dir == dir {
			return module, true
		}
	}
	return repositoryModule{}, false
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

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func cleanWorkspacePath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == "" {
		return "."
	}
	return cleaned
}

func containsPathSegment(path, want string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(path), "/"), want)
}

func firstPathSegment(path string) string {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	segment, _, _ := strings.Cut(path, "/")
	return segment
}

func importFamily(importPath string) string {
	if importPath == repositoryModulePath {
		return ""
	}
	return firstPathSegment(strings.TrimPrefix(importPath, repositoryModulePath+"/"))
}

func isRepositoryImport(path string) bool {
	return path == repositoryModulePath || strings.HasPrefix(path, repositoryModulePath+"/")
}

func isThirdPartyImport(path string) bool {
	first := firstPathSegment(path)
	return strings.Contains(first, ".")
}

func shouldSkipRepositoryDir(relative, name string) bool {
	if relative == "." {
		return false
	}
	if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
		return true
	}
	first := firstPathSegment(relative)
	return first == "app"
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
