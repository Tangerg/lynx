package repoarch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var documentedCapabilityModules = []string{
	"a2a",
	"agent",
	"etl",
	"eval",
	"mcp",
	"otel",
	"rag",
	"skills",
	"tools",
}

var documentationOnlyModuleRoots = []string{"core", "examples", "otel", "tools"}

// primaryAdoptionPackages are the packages a newcomer reads before writing any
// Scope code: the protocol, the ordinary client, and the suites a provider
// author must satisfy. modeltest belongs here because it is the only package an
// external implementor is required to read, and an undocumented fixture there
// costs every future provider rather than one caller.
var primaryAdoptionPackages = []string{"core/chat", "core/chatclient", "core/modeltest"}

// checkedExampleDeclarationSpan is deliberately coarse because one example
// explains a cooperating API slice, not one declaration. Sixty-four still
// forces another path before a module can hide a second package-sized vocabulary
// behind its original example.
const checkedExampleDeclarationSpan = 64

type packageDocumentation struct {
	hasOverview          bool
	exportedDeclarations int
}

type moduleDocumentation struct {
	packages        map[string]packageDocumentation
	checkedExamples int
}

func TestWorkspaceModulesKeepDocumentationEntryPoints(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)
	for _, modulePath := range slices.Sorted(maps.Keys(modules)) {
		module := modules[modulePath]
		t.Run(module.dir, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(root, filepath.FromSlash(module.dir), "doc.go")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("module %s is missing doc.go: %v", module.path, err)
			}
			if !info.Mode().IsRegular() || info.Size() == 0 {
				t.Fatalf("module %s has no readable content in doc.go", module.path)
			}
			assertPackageOverview(t, path)
			for _, retired := range []string{"README.md", "ARCHITECTURE.md"} {
				retiredPath := filepath.Join(root, filepath.FromSlash(module.dir), retired)
				if _, err := os.Stat(retiredPath); !os.IsNotExist(err) {
					t.Errorf("module %s keeps retired parallel documentation %s", module.path, retired)
				}
			}
		})
	}
}

// TestNamespaceDirectoriesKeepOneDocumentationEntry gates the directories that
// group modules without being modules themselves. They cannot carry a package
// comment, so README.md is their one entry point; a second parallel file would
// reintroduce exactly the split
// TestWorkspaceModulesKeepDocumentationEntryPoints removes from every module.
func TestNamespaceDirectoriesKeepOneDocumentationEntry(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	namespaces := discoverNamespaceDirectories(t, root)
	if len(namespaces) == 0 {
		t.Fatal("discovered no namespace directories; the gate would pass vacuously")
	}
	for _, namespace := range namespaces {
		t.Run(namespace, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, filepath.FromSlash(namespace))
			info, err := os.Stat(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatalf("namespace %s is missing README.md: %v", namespace, err)
			}
			if !info.Mode().IsRegular() || info.Size() == 0 {
				t.Fatalf("namespace %s has no readable content in README.md", namespace)
			}
			for _, retired := range []string{"ARCHITECTURE.md", "doc.go"} {
				if _, err := os.Stat(filepath.Join(dir, retired)); !os.IsNotExist(err) {
					t.Errorf("namespace %s keeps a second documentation entry %s", namespace, retired)
				}
			}
		})
	}
}

// discoverNamespaceDirectories returns the top-level directories that group
// workspace modules but declare no module of their own. Deriving them from the
// workspace rather than listing them keeps a newly added family from silently
// escaping the gate.
func discoverNamespaceDirectories(t *testing.T, root string) []string {
	t.Helper()
	namespaces := make(map[string]struct{})
	for _, module := range discoverModules(t, root) {
		prefix, _, nested := strings.Cut(filepath.ToSlash(module.dir), "/")
		if !nested {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, prefix, "go.mod")); os.IsNotExist(err) {
			namespaces[prefix] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(namespaces))
}

func TestRepositoryGuidanceHasOneCanonicalSource(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	claudePath := filepath.Join(root, "CLAUDE.md")
	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(claude), "@./AGENTS.md") != 1 {
		t.Error("CLAUDE.md must point to the canonical AGENTS.md exactly once")
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipRepositoryDir(filepath.ToSlash(relativePath), entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if relativePath == "AGENTS.md" || relativePath == "CLAUDE.md" {
			return nil
		}
		if entry.Name() == "AGENTS.md" || entry.Name() == "CLAUDE.md" {
			t.Errorf("%s duplicates repository guidance; put package contracts in GoDoc", filepath.ToSlash(relativePath))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRootReadmeKeepsDirectChatPath protects the ordinary library entry from
// being displaced by the more capable managed Agent path.
func TestRootReadmeKeepsDirectChatPath(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile(filepath.Join(repositoryRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(readme)
	for _, required := range []string{
		"```go",
		"github.com/Tangerg/scope/core/chatclient",
		"chatclient.New(",
		"client.Call(",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("README.md is missing direct chat entry %q", required)
		}
	}
}

// TestPrimaryAdoptionPackagesDocumentEntryDeclarations keeps the protocol and
// ordinary client discoverable on pkg.go.dev. Methods inherit their owning
// type's contract. Self-describing error sentinels stay exempt because a
// mandatory restatement adds no contract; sentinels with recovery or control-
// flow semantics still require human-authored GoDoc.
func TestPrimaryAdoptionPackagesDocumentEntryDeclarations(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relativePackage := range primaryAdoptionPackages {
		t.Run(relativePackage, func(t *testing.T) {
			t.Parallel()
			assertExportedEntryDeclarationsDocumented(
				t,
				filepath.Join(root, filepath.FromSlash(relativePackage)),
			)
		})
	}
}

// TestWorkspaceModulesDocumentEntryDeclarations extends the primary-adoption
// rule to every published package. A provider or backend is chosen from
// pkg.go.dev as often as Core is, so leaving its construction path undocumented
// costs the reader exactly where the decision is made. Methods stay exempt for
// the same reason as above, and internal packages are excluded because they are
// not part of anyone's adoption path.
func TestWorkspaceModulesDocumentEntryDeclarations(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	modules := discoverModules(t, root)
	for _, modulePath := range slices.Sorted(maps.Keys(modules)) {
		module := modules[modulePath]
		t.Run(module.dir, func(t *testing.T) {
			t.Parallel()
			moduleRoot := filepath.Join(root, filepath.FromSlash(module.dir))
			for _, directory := range publishedPackageDirectories(t, moduleRoot) {
				assertExportedEntryDeclarationsDocumented(t, directory)
			}
		})
	}
}

// publishedPackageDirectories returns the directories a consumer can import.
// Commands are skipped because a main package declares no contract, and
// internal trees are skipped because they cannot be imported from outside.
func publishedPackageDirectories(t *testing.T, moduleRoot string) []string {
	t.Helper()
	directories := make(map[string]struct{})
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleRoot && excludedDocumentationDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
		if err != nil {
			return err
		}
		if file.Name.Name == "main" {
			return nil
		}
		directories[filepath.Dir(path)] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(directories))
}

func assertExportedEntryDeclarationsDocumented(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil && declaration.Name.IsExported() && declaration.Doc == nil {
					t.Errorf("%s: exported function %s has no GoDoc", path, declaration.Name)
				}
			case *ast.GenDecl:
				assertExportedSpecificationsDocumented(t, path, declaration)
			}
		}
	}
}

// isErrorSentinelName recognizes Go's conventional exported error vocabulary
// without exempting unrelated names such as ErrorCollect. Requiring every
// self-describing sentinel to restate its name would reward filler; comments
// remain necessary when an error carries recovery or control-flow semantics.
func isErrorSentinelName(name string) bool {
	rest, found := strings.CutPrefix(name, "Err")
	if !found || rest == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return unicode.IsUpper(first)
}

func assertExportedSpecificationsDocumented(t *testing.T, path string, declaration *ast.GenDecl) {
	t.Helper()
	for _, specification := range declaration.Specs {
		var (
			exported bool
			name     string
			doc      *ast.CommentGroup
		)
		switch specification := specification.(type) {
		case *ast.TypeSpec:
			exported = specification.Name.IsExported()
			name = specification.Name.Name
			doc = specification.Doc
		case *ast.ValueSpec:
			for _, identifier := range specification.Names {
				if identifier.IsExported() && !isErrorSentinelName(identifier.Name) {
					exported = true
					name = identifier.Name
					break
				}
			}
			doc = specification.Doc
		}
		if exported && declaration.Doc == nil && doc == nil {
			t.Errorf("%s: exported declaration %s has no GoDoc", path, name)
		}
	}
}

// TestDocumentationOnlyModuleRootsStayDocumentationOnly keeps an overview from
// becoming the second public entry for capabilities owned by child packages.
func TestDocumentationOnlyModuleRootsStayDocumentationOnly(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relative := range documentationOnlyModuleRoots {
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			directory := filepath.Join(root, relative)
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				if entry.Name() != "doc.go" {
					t.Errorf("documentation-only module root %s contains production file %s", relative, entry.Name())
				}
			}

			path := filepath.Join(directory, "doc.go")
			file := assertPackageOverview(t, path)
			if len(file.Decls) != 0 {
				t.Errorf("%s/doc.go declares production API or implementation", relative)
			}
		})
	}
}

func assertPackageOverview(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if file.Doc == nil || !strings.HasPrefix(strings.TrimSpace(file.Doc.Text()), "Package "+file.Name.Name) {
		t.Errorf("%s has no package overview", path)
	}
	return file
}

// TestCapabilityModulesDocumentTheirPublicSurface keeps documentation
// discovery coupled to package discovery: new public package directories fail
// until their ownership is stated, and every capability module must retain an
// number of executable ordinary-path examples that grows with its public
// surface.
func TestCapabilityModulesDocumentTheirPublicSurface(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	for _, relativeModule := range documentedCapabilityModules {
		relativeModule := relativeModule
		t.Run(relativeModule, func(t *testing.T) {
			t.Parallel()
			moduleRoot := filepath.Join(root, filepath.FromSlash(relativeModule))
			documentation := inspectModuleDocumentation(t, moduleRoot)
			if len(documentation.packages) == 0 {
				t.Fatalf("capability module %s has no public packages", relativeModule)
			}
			exportedDeclarations := 0
			for directory, packageDocumentation := range documentation.packages {
				if !packageDocumentation.hasOverview {
					t.Errorf("public package %s has no Package comment in production code", filepath.ToSlash(directory))
				}
				exportedDeclarations += packageDocumentation.exportedDeclarations
			}
			requiredExamples := requiredCheckedExamples(exportedDeclarations)
			if documentation.checkedExamples < requiredExamples {
				t.Errorf(
					"capability module %s has %d checked Go examples for %d exported declarations; need at least %d",
					relativeModule,
					documentation.checkedExamples,
					exportedDeclarations,
					requiredExamples,
				)
			}
		})
	}
}

func inspectModuleDocumentation(t *testing.T, moduleRoot string) moduleDocumentation {
	t.Helper()
	documentation := moduleDocumentation{packages: make(map[string]packageDocumentation)}
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleRoot && excludedDocumentationDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			documentation.checkedExamples += countCheckedExamples(file)
			return nil
		}
		if file.Name.Name == "main" {
			return nil
		}
		directory := filepath.Dir(path)
		packageDocumentation := documentation.packages[directory]
		if file.Doc != nil && strings.HasPrefix(strings.TrimSpace(file.Doc.Text()), "Package "+file.Name.Name) {
			packageDocumentation.hasOverview = true
		}
		packageDocumentation.exportedDeclarations += countExportedDeclarations(file)
		documentation.packages[directory] = packageDocumentation
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return documentation
}

func requiredCheckedExamples(exportedDeclarations int) int {
	if exportedDeclarations == 0 {
		return 0
	}
	return (exportedDeclarations + checkedExampleDeclarationSpan - 1) / checkedExampleDeclarationSpan
}

// countExportedDeclarations measures package-level vocabulary and construction
// paths. Methods and individual names in one value declaration are deliberately
// not counted again because a checked example teaches them through their owning
// type or vocabulary as one cooperating API slice.
func countExportedDeclarations(file *ast.File) int {
	count := 0
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv == nil && declaration.Name.IsExported() {
				count++
			}
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				switch specification := specification.(type) {
				case *ast.TypeSpec:
					if specification.Name.IsExported() {
						count++
					}
				case *ast.ValueSpec:
					exported := false
					for _, name := range specification.Names {
						if name.IsExported() {
							exported = true
							break
						}
					}
					if exported {
						count++
					}
				}
			}
		}
	}
	return count
}

func TestRequiredCheckedExamplesScalesWithPublicSurface(t *testing.T) {
	t.Parallel()
	tests := []struct {
		declarations int
		want         int
	}{
		{declarations: 0, want: 0},
		{declarations: 1, want: 1},
		{declarations: checkedExampleDeclarationSpan, want: 1},
		{declarations: checkedExampleDeclarationSpan + 1, want: 2},
	}
	for _, test := range tests {
		if got := requiredCheckedExamples(test.declarations); got != test.want {
			t.Errorf("requiredCheckedExamples(%d) = %d, want %d", test.declarations, got, test.want)
		}
	}
}

func excludedDocumentationDirectory(name string) bool {
	return name == "examples" || name == "internal" || name == "testdata" || strings.HasPrefix(name, ".")
}

func countCheckedExamples(file *ast.File) int {
	count := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || !strings.HasPrefix(function.Name.Name, "Example") {
			continue
		}
		for _, comment := range file.Comments {
			if comment.Pos() < function.Body.Pos() || comment.End() > function.Body.End() {
				continue
			}
			text := strings.TrimSpace(comment.Text())
			if strings.HasPrefix(text, "Output:") || strings.HasPrefix(text, "Unordered output:") {
				count++
				break
			}
		}
	}
	return count
}
